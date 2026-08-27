package redfish

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/certificate"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

const defaultConnectionTestTimeout = 5 * time.Second

// environment provides access to the internal API of IncusOS.
type environment interface {
	GetSecureBootCertificates(ctx context.Context) (incusosapi.InternalSecureBootCertificates, error)
}

type redfish struct {
	connectionTestTimeout time.Duration
	env                   environment
}

var _ provisioning.BMCServerClientPort = redfish{}

type Option func(r *redfish)

func WithConnectionTestTimeout(timeout time.Duration) Option {
	return func(r *redfish) {
		r.connectionTestTimeout = timeout
	}
}

// WithSecureBootCertificates defines the source for the certificates, which are
// written to the respective secure boot databases (e.g. "KEK", "DB") when
// applying the secure boot certificates.
func WithSecureBootCertificates(env environment) Option {
	return func(r *redfish) {
		r.env = env
	}
}

func New(opts ...Option) redfish {
	r := redfish{
		connectionTestTimeout: defaultConnectionTestTimeout,
	}

	for _, opt := range opts {
		opt(&r)
	}

	return r
}

func (r redfish) getClient(ctx context.Context, server provisioning.Server) (_ *gofish.APIClient, logout func(), _ error) {
	if transaction.IsActive(ctx) {
		slog.WarnContext(ctx, "Redfish API call inside of a transaction", logger.AddStacktrace())
	}

	httpClient := &http.Client{}

	if server.BMCConfig.Certificate != "" {
		bmcServerCert, err := certificate.Decode([]byte(server.BMCConfig.Certificate))
		if err != nil {
			return nil, nil, fmt.Errorf("Invalid remote certificate: %w", err)
		}

		tlsConfig := &tls.Config{}
		if len(bmcServerCert.DNSNames) == 0 && len(bmcServerCert.IPAddresses) == 0 {
			// The certificate does not has any SAN, so we can only verify the connection based on the certificates fingerprint.
			sum := sha256.Sum256(bmcServerCert.Raw)
			trustedFingerprint := hex.EncodeToString(sum[:])

			tlsConfig.InsecureSkipVerify = true
			tlsConfig.VerifyConnection = func(cs tls.ConnectionState) error {
				// Verify the connection based on fingerprint comparison between the
				// BMC's and the trusted certificate.
				sum := sha256.Sum256(cs.PeerCertificates[0].Raw)
				fp := hex.EncodeToString(sum[:])

				if trustedFingerprint != fp {
					return fmt.Errorf("Certificate fingerprint mismatch for %q: expected %s, got %s (reissued cert?)", server.BMCConfig.Endpoint, trustedFingerprint, fp)
				}

				return nil
			}
		} else {
			// Make it a valid RootCA, create a new pool and add the certificate as root.
			bmcServerCert.IsCA = true
			bmcServerCert.KeyUsage = x509.KeyUsageCertSign

			pool := x509.NewCertPool()
			pool.AddCert(bmcServerCert)

			serverName := ""
			if len(bmcServerCert.IPAddresses) > 0 {
				serverName = bmcServerCert.IPAddresses[0].String()
			}

			if len(bmcServerCert.DNSNames) > 0 {
				serverName = bmcServerCert.DNSNames[0]
			}

			tlsConfig.RootCAs = pool
			tlsConfig.ServerName = serverName
		}

		httpClient.Transport = &http.Transport{
			TLSClientConfig: tlsConfig,

			ExpectContinueTimeout: 30 * time.Second,
			ResponseHeaderTimeout: 3600 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
		}
	}

	endpoint := server.BMCConfig.Endpoint
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil {
		return nil, nil, err
	}

	if parsedEndpoint.Port() == "443" {
		parsedEndpoint.Host = net.JoinHostPort(parsedEndpoint.Hostname(), "")
		endpoint = parsedEndpoint.String()
	}

	c, err := gofish.ConnectContext(ctx, gofish.ClientConfig{
		Endpoint:   endpoint,
		Username:   server.BMCConfig.Username,
		Password:   server.BMCConfig.Password,
		HTTPClient: httpClient,
		BasicAuth:  true,
		// DumpWriter: os.Stdout,
	})
	if err != nil {
		return nil, nil, err
	}

	return c, c.Logout, nil
}

func (r redfish) ConnectionTest(ctx context.Context, server provisioning.Server) (certificatePEM string, _ error) {
	var certPEM string

	ctx, cancel := context.WithTimeout(ctx, r.connectionTestTimeout)
	defer cancel()

	if server.BMCConfig.AutoPinCertificate && server.BMCConfig.Certificate == "" {
		cert, err := getRemoteCertificate(ctx, server.BMCConfig.Endpoint)
		if err != nil {
			return certPEM, fmt.Errorf("Failed to get remote certificate from BMC during connection test: %w", err)
		}

		certPEM = certificate.EncodeToPEM(cert.Raw)
		server.BMCConfig.Certificate = certPEM
	}

	_, logout, err := r.getClient(ctx, server)
	if err != nil {
		return certPEM, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	return certPEM, nil
}

func getRemoteCertificate(ctx context.Context, address string) (*x509.Certificate, error) {
	parsedAddress, err := url.Parse(address)
	if err != nil {
		return nil, err
	}

	if parsedAddress.Port() == "" {
		parsedAddress.Host = net.JoinHostPort(parsedAddress.Hostname(), "443")
	}

	dialer := tls.Dialer{
		Config: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	conn, err := dialer.DialContext(ctx, "tcp", parsedAddress.Host)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = conn.Close()
	}()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return nil, errors.New("Unable to read remote TLS certificate, not a TLS connection")
	}

	if len(tlsConn.ConnectionState().PeerCertificates) == 0 {
		return nil, errors.New("Unable to read remote TLS certificate")
	}

	return tlsConn.ConnectionState().PeerCertificates[0], nil
}

func (r redfish) GetData(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return api.BMCData{}, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	log := slog.With(slog.String("endpoint", server.BMCConfig.Endpoint))

	// The following data is collected from the BMC on a best effort basis.
	// Errors are logged as warnings and the affected data is left at its zero value.
	system, err := getFirstSystem(client)
	if err != nil {
		log.WarnContext(ctx, "Failed to get BMC system", logger.Err(err))
	}

	manager, err := getFirstManager(client)
	if err != nil {
		log.WarnContext(ctx, "Failed to get BMC manager", logger.Err(err))
	}

	bmcData := api.BMCData{
		BMCProtocol:        "Redfish",
		BMCProtocolVersion: client.Service.RedfishVersion,
		BMCVendor:          client.Service.Vendor,
		LastUpdated:        time.Now(),
	}

	if system != nil {
		bmcData.ServerManufacturer = system.Manufacturer
		bmcData.ServerModel = system.Model
		bmcData.ServerSubModel = system.SubModel
		bmcData.ServerUUID = system.UUID
		bmcData.ServerAssetTag = system.AssetTag
		bmcData.ServerHostName = system.HostName
		bmcData.ServerSKU = system.SKU
		bmcData.ServerSerialNumber = system.SerialNumber
		bmcData.ServerBIOSVersion = system.BiosVersion
		bmcData.ServerPowerState = string(system.PowerState)
		bmcData.ServerHealthStatus = string(system.Status.Health)

		bmcData.ServerLocationIndicatorActive = system.IndicatorLED == schemas.BlinkingIndicatorLED || system.IndicatorLED == schemas.LitIndicatorLED // nolint: staticcheck // ignore deprecated property warning.
		if system.LocationIndicatorActive != nil {
			bmcData.ServerLocationIndicatorActive = *system.LocationIndicatorActive
		}

		bmcData.ServerLastResetTime = parseTimestamp(ctx, log, "LastResetTime", system.LastResetTime)

		bmcData.ServerBootProgress = api.BMCBootProgress{
			LastState:     string(system.BootProgress.LastState),
			LastStateTime: parseTimestamp(ctx, log, "BootProgress/LastStateTime", system.BootProgress.LastStateTime),
			OEMLastState:  system.BootProgress.OEMLastState,
		}

		if system.BootProgress.LastBootTimeSeconds != nil {
			bmcData.ServerBootProgress.LastBootTimeSeconds = *system.BootProgress.LastBootTimeSeconds
		}
	}

	if manager != nil {
		bmcData.BMCModel = manager.Model
		bmcData.BMCFirmwareVersion = manager.FirmwareVersion
		bmcData.BMCServiceIdentification = manager.ServiceIdentification
	}

	if system != nil {
		processor, err := getFirstProcessor(system)
		if err != nil {
			log.WarnContext(ctx, "Failed to get first processor of BMC system", logger.Err(err))
		}

		if processor != nil {
			bmcData.ServerProcessorManufacturer = processor.Manufacturer
			bmcData.ServerProcessorArchitecture = string(processor.ProcessorArchitecture)
			bmcData.ServerProcessorInstructionSet = string(processor.InstructionSet)
		}

		if system.ProcessorSummary.Count != nil {
			bmcData.ServerCPUSockets = int(*system.ProcessorSummary.Count)
		}

		hasTPM, err := hasTrustedModule(system)
		if err != nil {
			log.WarnContext(ctx, "Failed to get trusted modules of BMC system", logger.Err(err))
		}

		bmcData.ServerHasTPM = hasTPM
	}

	if system != nil {
		bios, err := system.Bios()
		if err != nil {
			log.WarnContext(ctx, "Failed to get BIOS of BMC system", logger.Err(err))
		}

		if bios != nil {
			bmcData.ServerBIOSAttributes = bios.Attributes
		}
	}

	virtualMedia, err := getVirtualMedia(system, manager)
	if err != nil {
		log.WarnContext(ctx, "Failed to get virtual media of BMC", logger.Err(err))
	}

	bmcData.VirtualMedia = virtualMedia

	return bmcData, nil
}

func getFirstChassis(client *gofish.APIClient) (*schemas.Chassis, error) {
	chassis, err := client.Service.Chassis()
	if err != nil {
		return nil, fmt.Errorf("Failed to get BMC chassis: %w", err)
	}

	if len(chassis) == 0 {
		return nil, fmt.Errorf("No BMC chassis found: %w", domain.ErrNotFound)
	}

	sort.Slice(chassis, func(i, j int) bool { return chassis[i].ID < chassis[j].ID })

	return chassis[0], nil
}

func getFirstManager(client *gofish.APIClient) (*schemas.Manager, error) {
	managers, err := client.Service.Managers()
	if err != nil {
		return nil, fmt.Errorf("Failed to get BMC managers: %w", err)
	}

	if len(managers) == 0 {
		return nil, fmt.Errorf("No BMC managers found: %w", domain.ErrNotFound)
	}

	sort.Slice(managers, func(i, j int) bool { return managers[i].ID < managers[j].ID })

	return managers[0], nil
}

func getFirstSystem(client *gofish.APIClient) (*schemas.ComputerSystem, error) {
	systems, err := client.Service.Systems()
	if err != nil {
		return nil, fmt.Errorf("Failed to get BMC systems: %w", err)
	}

	if len(systems) == 0 {
		return nil, fmt.Errorf("No BMC systems found: %w", domain.ErrNotFound)
	}

	sort.Slice(systems, func(i, j int) bool { return systems[i].ID < systems[j].ID })

	return systems[0], nil
}

// getVirtualMedia returns all virtual media reported by the BMC, combining
// entries from both the system and the manager, keyed by "<service>:<redfish-id>"
// (e.g. "system:1").
func getVirtualMedia(system *schemas.ComputerSystem, manager *schemas.Manager) (map[string]api.BMCVirtualMedia, error) {
	var result map[string]api.BMCVirtualMedia

	if system != nil {
		systemVirtualMedia, err := system.VirtualMedia()
		if err != nil {
			return nil, fmt.Errorf("Failed to get virtual media of BMC system: %w", err)
		}

		result = convertVirtualMedia(result, "system", systemVirtualMedia)
	}

	if manager != nil {
		managerVirtualMedia, err := manager.VirtualMedia()
		if err != nil {
			return nil, fmt.Errorf("Failed to get virtual media of BMC manager: %w", err)
		}

		result = convertVirtualMedia(result, "manager", managerVirtualMedia)
	}

	return result, nil
}

func convertVirtualMedia(result map[string]api.BMCVirtualMedia, service string, virtualMedia []*schemas.VirtualMedia) map[string]api.BMCVirtualMedia {
	for _, vm := range virtualMedia {
		mediaTypes := make([]string, 0, len(vm.MediaTypes))
		for _, mediaType := range vm.MediaTypes {
			mediaTypes = append(mediaTypes, string(mediaType))
		}

		id := service + ":" + vm.ID

		if result == nil {
			result = map[string]api.BMCVirtualMedia{}
		}

		result[id] = api.BMCVirtualMedia{
			ID:                   id,
			Inserted:             ptr.From(vm.Inserted),
			WriteProtected:       ptr.From(vm.WriteProtected),
			Image:                vm.Image,
			ImageName:            vm.ImageName,
			ConnectedVia:         string(vm.ConnectedVia),
			Status:               string(vm.Status.Health),
			MediaTypes:           mediaTypes,
			TransferMethod:       string(vm.TransferMethod),
			TransferProtocolType: string(vm.TransferProtocolType),
		}
	}

	return result
}

// getBIOSSettingsApplyTimes fetches the BIOS resource and returns the apply
// times the BMC actually declared support for.
// TODO: replace when https://github.com/stmcginnis/gofish/issues/551 is resolved.
func getBIOSSettingsApplyTimes(client *gofish.APIClient, biosODataID string) ([]schemas.SettingsApplyTime, error) {
	resp, err := client.Get(biosODataID)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Settings schemas.Settings `json:"@Redfish.Settings"`
	}

	err = json.Unmarshal(body, &raw)
	if err != nil {
		return nil, err
	}

	return raw.Settings.SupportedApplyTimes, nil
}

// getBIOSAttributeRegistry resolves and fetches the BIOS attribute registry
// identified by registryName (schemas.Bios.AttributeRegistry), which lists
// the acceptable values for each BIOS attribute. Returns nil, nil if no
// matching registry is published by the BMC.
func getBIOSAttributeRegistry(client *gofish.APIClient, registryName string) (*schemas.AttributeRegistry, error) {
	if registryName == "" {
		return nil, nil
	}

	files, err := client.Service.Registries()
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if file.Registry != registryName && file.ID != registryName {
			continue
		}

		for _, location := range file.Location {
			if location.URI == "" {
				continue
			}

			return schemas.GetAttributeRegistry(client, location.URI)
		}
	}

	return nil, nil
}

// findBIOSAttribute looks up the attribute with the given name in the BIOS
// attribute registry.
func findBIOSAttribute(registry *schemas.AttributeRegistry, attributeName string) (schemas.Attributes, bool) {
	if registry == nil {
		return schemas.Attributes{}, false
	}

	for _, attribute := range registry.RegistryEntries.Attributes {
		if attribute.AttributeName == attributeName {
			return attribute, true
		}
	}

	return schemas.Attributes{}, false
}

// attributeValueNames returns the enumeration values names declared as
// acceptable for the given attribute. Empty if the attribute is not an
// enumeration.
func attributeValueNames(attribute schemas.Attributes) []string {
	values := make([]string, 0, len(attribute.Value))
	for _, value := range attribute.Value {
		values = append(values, value.ValueName)
	}

	return values
}

// biosAttributeAcceptableValues returns the enumeration values the BIOS
// attribute registry declares as acceptable for the given attribute name, or
// nil if the attribute is not an enumeration or not found in the registry.
func biosAttributeAcceptableValues(registry *schemas.AttributeRegistry, attributeName string) []string {
	attribute, ok := findBIOSAttribute(registry, attributeName)
	if !ok {
		return nil
	}

	return attributeValueNames(attribute)
}

// newBIOSAttribute returns the API representation of the BIOS attribute with
// the given name and current value, enriched with the metadata from the BIOS
// attribute registry if the registry describes the attribute.
func newBIOSAttribute(registry *schemas.AttributeRegistry, name string, value any) api.BIOSAttribute {
	attribute, ok := findBIOSAttribute(registry, name)
	if !ok {
		return api.BIOSAttribute{
			Name:         name,
			CurrentValue: value,
		}
	}

	return api.BIOSAttribute{
		Name:             name,
		Type:             string(attribute.Type),
		CurrentValue:     value,
		LowerBound:       ptr.ToInt64(attribute.LowerBound),
		UpperBound:       ptr.ToInt64(attribute.UpperBound),
		MinLength:        ptr.ToInt64(attribute.MinLength),
		MaxLength:        ptr.ToInt64(attribute.MaxLength),
		AcceptableValues: attributeValueNames(attribute),
	}
}

// describeBIOSAttributesError turns a Redfish client-error (4xx) returned
// while applying BIOS attributes into a domain.ErrValidation with a human
// readable message, enriched with the acceptable values from the BIOS
// attribute registry where possible. If err does not carry structured
// Redfish error information, or is not a client error, it is returned
// unchanged.
func describeBIOSAttributesError(ctx context.Context, client *gofish.APIClient, biosAttributeRegistry string, err error) error {
	var redfishErr *schemas.Error

	if !errors.As(err, &redfishErr) || len(redfishErr.ExtendedInfos) == 0 {
		return err
	}

	if redfishErr.HTTPReturnedStatusCode < 400 || redfishErr.HTTPReturnedStatusCode >= 500 {
		return err
	}

	registry, registryErr := getBIOSAttributeRegistry(client, biosAttributeRegistry)
	if registryErr != nil {
		slog.WarnContext(ctx, "Failed to get BIOS attribute registry to enrich error message", logger.Err(registryErr))
	}

	messages := make([]string, 0, len(redfishErr.ExtendedInfos))
	for _, info := range redfishErr.ExtendedInfos {
		// Message is optional, fall back to the message registry identifier.
		message := info.Message
		if message == "" {
			message = info.MessageID
		}

		if message == "" {
			continue
		}

		for _, property := range info.RelatedProperties {
			values := biosAttributeAcceptableValues(registry, path.Base(property))
			if len(values) == 0 {
				continue
			}

			message = fmt.Sprintf("%s Acceptable values: %s.", message, strings.Join(values, ", "))
		}

		messages = append(messages, message)
	}

	// Without any human readable message, the original error carries more
	// information than an empty validation error would.
	if len(messages) == 0 {
		return err
	}

	return domain.NewValidationErrf("%s", strings.Join(messages, " "))
}

// hasTrustedModule reports, whether the BMC system has a trusted platform
// module installed. Slots without a module are reported by the BMC in the state
// absent and therefore do not count.
//
// Both sources, trusted modules (now deprecated) and trusted components
// are considered.
func hasTrustedModule(system *schemas.ComputerSystem) (bool, error) {
	for _, trustedModule := range system.TrustedModules { // nolint: staticcheck // ignore deprecated property warning.
		if trustedModule.Status.State != schemas.AbsentState {
			return true, nil
		}
	}

	trustedComponents, err := system.TrustedComponents()
	if err != nil {
		return false, fmt.Errorf("Failed to get trusted components of BMC system: %w", err)
	}

	for _, trustedComponent := range trustedComponents {
		if trustedComponent.Status.State != schemas.AbsentState {
			return true, nil
		}
	}

	return false, nil
}

func getFirstProcessor(system *schemas.ComputerSystem) (*schemas.Processor, error) {
	processors, err := system.Processors()
	if err != nil {
		return nil, fmt.Errorf("Failed to get processors of BMC system: %w", err)
	}

	if len(processors) == 0 {
		return nil, fmt.Errorf("No processors found for the BMC system")
	}

	sort.Slice(processors, func(i, j int) bool { return processors[i].ID < processors[j].ID })

	return processors[0], nil
}

func (r redfish) ServerPowerOn(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
	resetType := schemas.OnResetType
	if force {
		resetType = schemas.ForceOnResetType
	}

	return r.performReset(ctx, server, resetType)
}

func (r redfish) ServerPowerOff(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
	resetType := schemas.GracefulShutdownResetType
	if force {
		resetType = schemas.ForceOffResetType
	}

	return r.performReset(ctx, server, resetType)
}

func (r redfish) ServerRestart(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
	resetType := schemas.GracefulRestartResetType
	if force {
		resetType = schemas.ForceRestartResetType
	}

	return r.performReset(ctx, server, resetType)
}

func (r redfish) ServerSetLocationIndicator(ctx context.Context, server provisioning.Server, active bool) error {
	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	system, err := getFirstSystem(client)
	if err != nil {
		return fmt.Errorf("Failed get BMC system: %w", err)
	}

	switch {
	case system.LocationIndicatorActive != nil:
		system.LocationIndicatorActive = &active

	case system.IndicatorLED != "": // nolint: staticcheck // ignore deprecated property warning.
		// Fall back to the deprecated IndicatorLED property, if the BMC does not
		// report LocationIndicatorActive.
		indicatorLED := schemas.OffIndicatorLED
		if active {
			indicatorLED = schemas.LitIndicatorLED
		}

		system.IndicatorLED = indicatorLED // nolint: staticcheck // ignore deprecated property warning.

	default:
		return fmt.Errorf("The BMC does not support the location indicator LED: %w", domain.ErrOperationNotPermitted)
	}

	// Update only performs a request, if the desired state differs from the
	// state reported by the BMC.
	err = system.Update()
	if err != nil {
		return fmt.Errorf("Failed to set location indicator LED via BMC: %w", err)
	}

	return nil
}

const defaultWaitForTaskRetryAfter = 2 * time.Second

// taskState reports the state of the task behind uri and, while it is still
// running, how long the BMC asks to wait before the next poll.
func taskState(client *gofish.APIClient, uri string) (api.BMCTaskState, time.Duration, error) {
	resp, err := client.Get(uri)
	if err != nil {
		if isTaskMonitorGone(err) {
			return api.BMCTaskStateUnknown, 0, nil
		}

		return api.BMCTaskStateUnknown, 0, wrapRedfishError(err)
	}

	resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusAccepted: // still running
		return api.BMCTaskStateRunning, taskRetryAfter(resp), nil

	case http.StatusOK, http.StatusCreated: // task finished
		return api.BMCTaskStateCompleted, 0, nil

	default:
		return api.BMCTaskStateUnknown, 0, fmt.Errorf("Unexpected status %d polling %s", resp.StatusCode, uri)
	}
}

// taskRetryAfter returns the poll interval the BMC asks for.
func taskRetryAfter(resp *http.Response) time.Duration {
	ra := resp.Header.Get("Retry-After")
	if ra == "" {
		return defaultWaitForTaskRetryAfter
	}

	secs, err := strconv.Atoi(ra)
	if err != nil {
		return defaultWaitForTaskRetryAfter
	}

	return time.Duration(secs) * time.Second
}

func (r redfish) TaskState(ctx context.Context, server provisioning.Server, taskMonitor *provisioning.BMCTaskMonitor) (api.BMCTaskState, error) {
	if taskMonitor == nil || taskMonitor.URI == "" {
		return api.BMCTaskStateUnknown, nil
	}

	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return api.BMCTaskStateUnknown, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	state, _, err := taskState(client, taskMonitor.URI)
	if err != nil {
		return api.BMCTaskStateUnknown, err
	}

	return state, nil
}

func (r redfish) WaitForTask(ctx context.Context, server provisioning.Server, taskMonitor *provisioning.BMCTaskMonitor) error {
	if taskMonitor == nil {
		return nil
	}

	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	uri := taskMonitor.URI

	for {
		err := ctx.Err()
		if err != nil {
			return fmt.Errorf("Waiting for task %s: %w", uri, err)
		}

		state, retryAfter, err := taskState(client, uri)
		if err != nil {
			return err
		}

		switch state {
		case api.BMCTaskStateRunning: // keep polling

		case api.BMCTaskStateCompleted:
			return nil

		case api.BMCTaskStateUnknown:
			return fmt.Errorf("The BMC does not know the task %s (any more)", uri)
		}

		select {
		case <-time.After(retryAfter):
			continue

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (r redfish) ApplyBIOSAttributes(ctx context.Context, server provisioning.Server, attributes map[string]any) (*provisioning.BMCTaskMonitor, error) {
	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	system, err := getFirstSystem(client)
	if err != nil {
		return nil, fmt.Errorf("Failed get BMC system: %w", err)
	}

	bios, err := system.Bios()
	if err != nil {
		return nil, fmt.Errorf("Failed to get bios information: %w", err)
	}

	supportedApplyTimes, err := getBIOSSettingsApplyTimes(client, bios.ODataID)
	if err != nil {
		return nil, fmt.Errorf("Failed to get bios settings apply time capabilities: %w", err)
	}

	if !slices.Contains(supportedApplyTimes, schemas.OnResetSettingsApplyTime) {
		err = bios.UpdateBiosAttributes(schemas.SettingsAttributes(attributes))
		if err != nil {
			return nil, fmt.Errorf("Failed to apply bios attributes: %w", describeBIOSAttributesError(ctx, client, bios.AttributeRegistry, err))
		}

		return nil, nil
	}

	tm, err := bios.UpdateBiosAttributesApplyAtWithTask(schemas.SettingsAttributes(attributes), schemas.OnResetSettingsApplyTime)
	if err != nil {
		return nil, fmt.Errorf("Failed to apply bios attributes: %w", describeBIOSAttributesError(ctx, client, bios.AttributeRegistry, err))
	}

	if tm != nil {
		return &provisioning.BMCTaskMonitor{
			URI: tm.TaskMonitor,
		}, nil
	}

	return nil, nil
}

func (r redfish) BIOSAttributes(ctx context.Context, server provisioning.Server) ([]api.BIOSAttribute, error) {
	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	system, err := getFirstSystem(client)
	if err != nil {
		return nil, fmt.Errorf("Failed get BMC system: %w", err)
	}

	bios, err := system.Bios()
	if err != nil {
		return nil, fmt.Errorf("Failed to get bios information: %w", err)
	}

	registry, err := getBIOSAttributeRegistry(client, bios.AttributeRegistry)
	if err != nil {
		slog.WarnContext(ctx, "Failed to get BIOS attribute registry, returning BIOS attributes without registry metadata", logger.Err(err))
	}

	// The attributes reported by the BIOS are the source of truth, the registry
	// only adds metadata for those attributes it describes.
	attributes := make([]api.BIOSAttribute, 0, len(bios.Attributes))
	for name, value := range bios.Attributes {
		attributes = append(attributes, newBIOSAttribute(registry, name, value))
	}

	sort.Slice(attributes, func(i, j int) bool { return attributes[i].Name < attributes[j].Name })

	return attributes, nil
}

func (r redfish) BIOSAttribute(ctx context.Context, server provisioning.Server, attributeName string) (api.BIOSAttribute, error) {
	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return api.BIOSAttribute{}, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	system, err := getFirstSystem(client)
	if err != nil {
		return api.BIOSAttribute{}, fmt.Errorf("Failed get BMC system: %w", err)
	}

	bios, err := system.Bios()
	if err != nil {
		return api.BIOSAttribute{}, fmt.Errorf("Failed to get bios information: %w", err)
	}

	value, ok := bios.Attributes[attributeName]
	if !ok {
		return api.BIOSAttribute{}, fmt.Errorf("BIOS attribute %q not found: %w", attributeName, domain.ErrNotFound)
	}

	registry, err := getBIOSAttributeRegistry(client, bios.AttributeRegistry)
	if err != nil {
		slog.WarnContext(ctx, "Failed to get BIOS attribute registry, returning BIOS attribute without registry metadata", logger.Err(err))
	}

	return newBIOSAttribute(registry, attributeName, value), nil
}

func (r redfish) performReset(ctx context.Context, server provisioning.Server, resetType schemas.ResetType) (*provisioning.BMCTaskMonitor, error) {
	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	system, err := getFirstSystem(client)
	if err != nil {
		return nil, fmt.Errorf("Failed get BMC system: %w", err)
	}

	allowedResetTypes, err := system.GetSupportedResetTypes()
	if err != nil {
		return nil, fmt.Errorf("Failed to get supported reset types from BMC: %w", err)
	}

	if len(allowedResetTypes) > 0 && !slices.Contains(allowedResetTypes, resetType) {
		return nil, fmt.Errorf("Reset type %q is not supported by the BMC, supported types are: %v", resetType, allowedResetTypes)
	}

	taskMonitor, err := system.Reset(resetType)
	if err != nil {
		return nil, fmt.Errorf("Failed to perform BMC reset operation: %w", wrapRedfishError(err))
	}

	// If taskMonitor is nil, the BMC completed synchronously.
	if taskMonitor == nil {
		return nil, nil
	}

	return &provisioning.BMCTaskMonitor{
		URI: taskMonitor.TaskMonitor,
	}, nil
}

// virtualMediaSlot is a virtual media resource of a BMC, together with the
// message registry needed to make sense of what the BMC answers about it.
type virtualMediaSlot struct {
	*schemas.VirtualMedia

	registry *messageRegistry
}

func getVirtualMediaByID(client *gofish.APIClient, id string) (virtualMediaSlot, error) {
	service, redfishID, ok := strings.Cut(id, ":")
	if !ok || service == "" || redfishID == "" {
		return virtualMediaSlot{}, fmt.Errorf("Invalid virtual media ID %q, expected format %q (e.g. %q): %w", id, "<service>:<id>", "system:1", domain.ErrOperationNotPermitted)
	}

	var virtualMedias []*schemas.VirtualMedia

	var virtualMediaReturner interface {
		VirtualMedia() ([]*schemas.VirtualMedia, error)
	}
	var err error

	switch service {
	case "system":
		virtualMediaReturner, err = getFirstSystem(client)

	case "manager":
		virtualMediaReturner, err = getFirstManager(client)

	default:
		return virtualMediaSlot{}, fmt.Errorf("Unknown virtual media service %q in ID %q, expected %q or %q: %w", service, id, "system", "manager", domain.ErrOperationNotPermitted)
	}

	if err != nil {
		return virtualMediaSlot{}, fmt.Errorf("Failed to get BMC system: %w", wrapRedfishError(err))
	}

	virtualMedias, err = virtualMediaReturner.VirtualMedia()
	if err != nil {
		return virtualMediaSlot{}, fmt.Errorf("Failed to get virtual media from BMC system: %w", wrapRedfishError(err))
	}

	for _, vm := range virtualMedias {
		if vm.ID == redfishID {
			return virtualMediaSlot{VirtualMedia: vm, registry: newMessageRegistry(client)}, nil
		}
	}

	return virtualMediaSlot{}, fmt.Errorf("Virtual media %q not found on BMC: %w", id, domain.ErrNotFound)
}

func (r redfish) AttachMedia(ctx context.Context, server provisioning.Server, virtualMediaID string, mediaURL string, setBootDevice bool) (*provisioning.BMCTaskMonitor, error) {
	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	virtualMedia, err := getVirtualMediaByID(client, virtualMediaID)
	if err != nil {
		return nil, err
	}

	if virtualMediaHasMedia(virtualMedia) {
		return nil, fmt.Errorf("Virtual media %q already has media attached, detach the media first: %w", virtualMediaID, domain.ErrOperationNotPermitted)
	}

	err = checkMediaTypeSupported(virtualMedia, virtualMediaID, mediaURL)
	if err != nil {
		return nil, err
	}

	var system *schemas.ComputerSystem

	var bootSource schemas.BootSource

	mediaTypes := mediaTypesForImage(virtualMedia, mediaURL)

	if setBootDevice {
		system, err = getFirstSystem(client)
		if err != nil {
			return nil, fmt.Errorf("Failed to get BMC system: %w", err)
		}

		mediaTypes, err = bootableMediaTypes(system, mediaTypes, virtualMediaID)
		if err != nil {
			return nil, err
		}

		bootSource = bootSourceForVirtualMediaType[mediaTypes[0]]
	}

	trace, stopTrace := traceRequests(client)
	defer stopTrace()

	mediaType, taskMonitor, err := insertMedia(virtualMedia, mediaURL, mediaTypes)
	if err != nil {
		slog.DebugContext(
			ctx, "Attaching virtual media to BMC failed",
			slog.String("endpoint", server.BMCConfig.Endpoint),
			slog.String("virtual_media_id", virtualMediaID),
			slog.String("trace", trace.String()),
		)

		return nil, fmt.Errorf("Failed to attach media to BMC: %w", wrapRedfishError(err))
	}

	if setBootDevice {
		if mediaType != "" {
			bootSource = bootSourceForVirtualMediaType[mediaType]
		}

		err = overrideBootDevice(system, virtualMedia.registry, bootSource)
		if err != nil {
			slog.DebugContext(
				ctx, "Setting the boot device of the BMC to the attached virtual media failed",
				slog.String("endpoint", server.BMCConfig.Endpoint),
				slog.String("virtual_media_id", virtualMediaID),
				slog.String("trace", trace.String()),
			)

			_, ejectErr := ejectMedia(virtualMedia)
			if ejectErr != nil {
				slog.WarnContext(
					ctx, "Failed to detach the virtual media again after setting the boot device failed",
					logger.Err(ejectErr),
					slog.String("endpoint", server.BMCConfig.Endpoint),
					slog.String("virtual_media_id", virtualMediaID),
				)
			}

			return nil, fmt.Errorf("Failed to set boot device on BMC: %w", wrapRedfishError(err))
		}
	}

	// If taskMonitor is nil, the BMC completed synchronously.
	if taskMonitor == nil {
		return nil, nil
	}

	return &provisioning.BMCTaskMonitor{
		URI: taskMonitor.TaskMonitor,
	}, nil
}

func (r redfish) DetachMedia(ctx context.Context, server provisioning.Server, virtualMediaID string) (*provisioning.BMCTaskMonitor, error) {
	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	virtualMedia, err := getVirtualMediaByID(client, virtualMediaID)
	if err != nil {
		return nil, err
	}

	// No media attached, nothing to detach.
	if !virtualMediaHasMedia(virtualMedia) {
		return nil, nil
	}

	system, err := getFirstSystem(client)
	if err != nil {
		return nil, fmt.Errorf("Failed to get BMC system: %w", err)
	}

	trace, stopTrace := traceRequests(client)
	defer stopTrace()

	taskMonitor, err := ejectMedia(virtualMedia)
	if err != nil {
		slog.DebugContext(
			ctx, "Detaching virtual media from BMC failed",
			slog.String("endpoint", server.BMCConfig.Endpoint),
			slog.String("virtual_media_id", virtualMediaID),
			slog.String("trace", trace.String()),
		)

		return nil, fmt.Errorf("Failed to detach media from BMC: %w", wrapRedfishError(err))
	}

	restored, err := restoreDefaultBootDevice(system, virtualMedia.registry, bootSourcesForMediaTypes(virtualMedia.MediaTypes))
	if err != nil {
		slog.DebugContext(
			ctx, "Restoring the default boot device of the BMC after detaching virtual media failed",
			slog.String("endpoint", server.BMCConfig.Endpoint),
			slog.String("virtual_media_id", virtualMediaID),
			slog.String("trace", trace.String()),
		)

		return nil, fmt.Errorf("Failed to restore the default boot device on BMC: %w", wrapRedfishError(err))
	}

	if !restored {
		slog.DebugContext(
			ctx, "Left the boot device of the BMC alone, it does not point at the detached virtual media",
			slog.String("endpoint", server.BMCConfig.Endpoint),
			slog.String("virtual_media_id", virtualMediaID),
		)
	}

	// If taskMonitor is nil, the BMC completed synchronously.
	if taskMonitor == nil {
		return nil, nil
	}

	return &provisioning.BMCTaskMonitor{
		URI: taskMonitor.TaskMonitor,
	}, nil
}

func (r redfish) LogSources(ctx context.Context, server provisioning.Server) ([]string, error) {
	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	services := make(map[string]func() ([]*schemas.LogService, error), 3)

	chassis, err := getFirstChassis(client)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	if err == nil {
		services["chassis"] = chassis.LogServices
	}

	system, err := getFirstSystem(client)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	if err == nil {
		services["system"] = system.LogServices
	}

	manager, err := getFirstManager(client)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	if err == nil {
		services["manager"] = manager.LogServices
	}

	var logSources []string
	for name, getLogServices := range services {
		logServices, err := getLogServices()
		if err != nil {
			return nil, fmt.Errorf("Failed to get log services for %q: %w", name, err)
		}

		for _, logService := range logServices {
			logSources = append(logSources, name+"/"+logService.ID)
		}
	}

	sort.Strings(logSources)

	return logSources, nil
}

func (r redfish) LogEntriesBySource(ctx context.Context, server provisioning.Server, logSource string) ([]api.BMCLogEvent, error) {
	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	logSourceParts := strings.Split(logSource, "/")
	if len(logSourceParts) != 2 {
		return nil, fmt.Errorf(`Invalid log source %q, expect "service/logType"`, logSource)
	}

	service := logSourceParts[0]
	logType := logSourceParts[1]

	var getLogServices func() ([]*schemas.LogService, error)

	switch service {
	case "chassis":
		chassis, err := getFirstChassis(client)
		if err != nil {
			return nil, fmt.Errorf("Failed to get BMC chassis: %w", err)
		}

		getLogServices = chassis.LogServices

	case "system":
		system, err := getFirstSystem(client)
		if err != nil {
			return nil, fmt.Errorf("Failed to get BMC system: %w", err)
		}

		getLogServices = system.LogServices

	case "manager":
		manager, err := getFirstManager(client)
		if err != nil {
			return nil, fmt.Errorf("Failed to get BMC manager: %w", err)
		}

		getLogServices = manager.LogServices

	default:
		return nil, fmt.Errorf(`Invalid log source service %q, expect one of "chassis", "system" or "manager"`, service)
	}

	logServices, err := getLogServices()
	if err != nil {
		return nil, fmt.Errorf("Failed to get log services for %q: %w", logSource, err)
	}

	found := false
	var entries []*schemas.LogEntry
	for _, logService := range logServices {
		if logService.ID != logType {
			continue
		}

		entries, err = logService.Entries()
		if err != nil {
			return nil, fmt.Errorf("Failed to get log entries for %q: %w", logSource, err)
		}

		found = true
	}

	if !found {
		return nil, fmt.Errorf("Failed to find log type for %q", logSource)
	}

	bmcLogEvents := make([]api.BMCLogEvent, 0, len(entries))
	for _, entry := range entries {
		bmcLogEvents = append(bmcLogEvents, api.BMCLogEvent{
			EntryCode: string(entry.EntryCode),
			EntryType: string(entry.EntryType),
			Message:   entry.Message,
			Severity:  string(entry.Severity),
			Timestamp: parseTimestamp(ctx, slog.Default(), "LogEntry/Created,LogEntry/EventTimestamp", entry.Created, entry.EventTimestamp),
		})
	}

	// Sort newest to oldest, entries whose timestamp could not be parsed are moved to the end.
	sort.SliceStable(bmcLogEvents, func(i, j int) bool {
		iZero := bmcLogEvents[i].Timestamp.IsZero()
		jZero := bmcLogEvents[j].Timestamp.IsZero()
		if iZero != jZero {
			return jZero
		}

		return bmcLogEvents[i].Timestamp.After(bmcLogEvents[j].Timestamp)
	})

	return bmcLogEvents, nil
}

// timestampFormats holds the formats, in which BMCs report timestamps.
var timestampFormats = []string{time.RFC3339, time.RFC3339Nano}

// parseTimestamp returns the first timestamp, which successfully parses for one
// of BMC time formats. If none of the timestamps could be parsed with any
// of the formats, the zero time.Time is returned.
func parseTimestamp(ctx context.Context, log *slog.Logger, property string, timestamps ...string) time.Time {
	reported := false

	for _, timestamp := range timestamps {
		if timestamp == "" {
			continue
		}

		reported = true

		for _, format := range timestampFormats {
			t, err := time.Parse(format, timestamp)
			if err == nil {
				return t
			}
		}
	}

	if reported {
		log.WarnContext(ctx, "Failed to parse timestamp reported by BMC", slog.String("properties", property), slog.Any("timestamps", timestamps))
	}

	return time.Time{}
}
