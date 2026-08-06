package redfish

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/shared/api"
)

type redfish struct {
	secureBootCertificates map[string][]schemas.Certificate
}

var _ provisioning.BMCServerClientPort = redfish{}

type Option func(*redfish)

// WithSecureBootCertificates defines the certificates, which are written to the
// respective secure boot databases (e.g. "KEK", "DB") during setup of the
// secure boot certificates.
func WithSecureBootCertificates(certificates map[string][]schemas.Certificate) Option {
	return func(r *redfish) {
		r.secureBootCertificates = certificates
	}
}

func New(opts ...Option) redfish {
	r := redfish{}

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
		certBlock, _ := pem.Decode([]byte(server.BMCConfig.Certificate))
		if certBlock == nil {
			return nil, nil, errors.New("Invalid remote certificate")
		}

		var err error
		bmcServerCert, err := x509.ParseCertificate(certBlock.Bytes)
		if err != nil {
			return nil, nil, err
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

func (r redfish) ConnectionTest(ctx context.Context, server provisioning.Server) (certificate string, _ error) {
	var certPEM string

	if server.BMCConfig.AutoPinCertificate && server.BMCConfig.Certificate == "" {
		cert, err := getRemoteCertificate(server.BMCConfig.Endpoint)
		if err != nil {
			return certPEM, fmt.Errorf("Failed to get remote certificate from BMC during connection test: %w", err)
		}

		certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))
		server.BMCConfig.Certificate = certPEM
	}

	_, logout, err := r.getClient(ctx, server)
	if err != nil {
		return certPEM, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	return certPEM, nil
}

func getRemoteCertificate(address string) (*x509.Certificate, error) {
	parsedAddress, err := url.Parse(address)
	if err != nil {
		return nil, err
	}

	if parsedAddress.Port() == "" {
		parsedAddress.Host = net.JoinHostPort(parsedAddress.Hostname(), "443")
	}

	conn, err := tls.Dial("tcp", parsedAddress.Host, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = conn.Close()
	}()

	if len(conn.ConnectionState().PeerCertificates) == 0 {
		return nil, errors.New("Unable to read remote TLS certificate")
	}

	return conn.ConnectionState().PeerCertificates[0], nil
}

func (r redfish) GetData(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return api.BMCData{}, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	system, err := getFirstSystem(client)
	if err != nil {
		return api.BMCData{}, fmt.Errorf("Failed to get BMC system: %w", err)
	}

	manager, err := getFirstManager(client)
	if err != nil {
		return api.BMCData{}, fmt.Errorf("Failed to get BMC manager: %w", err)
	}

	processor, err := getFirstProcessor(system)
	if err != nil {
		return api.BMCData{}, fmt.Errorf("Failed to get first processor of BMC system: %w", err)
	}

	serverLocationIndicatorActive := system.IndicatorLED == schemas.BlinkingIndicatorLED || system.IndicatorLED == schemas.LitIndicatorLED // nolint: staticcheck // ignore deprecated property warning.
	if system.LocationIndicatorActive != nil {
		serverLocationIndicatorActive = *system.LocationIndicatorActive
	}

	return api.BMCData{
		BMCProtocol:                   "Redfish",
		BMCProtocolVersion:            client.Service.RedfishVersion,
		BMCVendor:                     client.Service.Vendor,
		BMCModel:                      manager.Model,
		BMCFirmwareVersion:            manager.FirmwareVersion,
		BMCServiceIdentification:      manager.ServiceIdentification,
		ServerManufacturer:            system.Manufacturer,
		ServerModel:                   system.Model,
		ServerSubModel:                system.SubModel,
		ServerUUID:                    system.UUID,
		ServerAssetTag:                system.AssetTag,
		ServerHostName:                system.HostName,
		ServerSKU:                     system.SKU,
		ServerSerialNumber:            system.SerialNumber,
		ServerBIOSVersion:             system.BiosVersion,
		ServerProcessorArchitecture:   string(processor.ProcessorArchitecture),
		ServerProcessorInstructionSet: string(processor.InstructionSet),
		ServerPowerState:              string(system.PowerState),
		ServerLocationIndicatorActive: serverLocationIndicatorActive,
		ServerHealthStatus:            string(system.Status.Health),
		LastUpdated:                   time.Now(),
	}, nil
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

const defaultWaitForTaskRetryAfter = 2 * time.Second

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

		resp, err := client.Get(uri)
		if err != nil {
			return err
		}

		resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusAccepted: // still running

		case http.StatusOK, http.StatusCreated: // task finished
			return nil

		default:
			return fmt.Errorf("Unexpected status %d polling %s", resp.StatusCode, uri)
		}

		wait := defaultWaitForTaskRetryAfter
		ra := resp.Header.Get("Retry-After")
		if ra != "" {
			secs, err := strconv.Atoi(ra)
			if err == nil {
				wait = time.Duration(secs) * time.Second
			}
		}

		select {
		case <-time.After(wait):
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
			return nil, fmt.Errorf("Failed to apply bios attributes: %w", err)
		}

		return nil, nil
	}

	tm, err := bios.UpdateBiosAttributesApplyAtWithTask(schemas.SettingsAttributes(attributes), schemas.OnResetSettingsApplyTime)
	if err != nil {
		return nil, fmt.Errorf("Failed to apply bios attributes: %w", err)
	}

	if tm != nil {
		return &provisioning.BMCTaskMonitor{
			URI: tm.TaskMonitor,
		}, nil
	}

	return nil, nil
}

func (r redfish) SetupSecureBootCertificates(ctx context.Context, server provisioning.Server) error {
	if len(r.secureBootCertificates) == 0 {
		return fmt.Errorf("Setup of secure boot certificates is not supported, no certificates are configured: %w", domain.ErrOperationNotPermitted)
	}

	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	system, err := getFirstSystem(client)
	if err != nil {
		return fmt.Errorf("Failed get BMC system: %w", err)
	}

	secureBoot, err := system.SecureBoot()
	if err != nil {
		return fmt.Errorf("Failed to get secure boot information: %w", err)
	}

	secureBootDatabases, err := secureBoot.SecureBootDatabases()
	if err != nil {
		return fmt.Errorf("Failed to get secure boot databases: %w", err)
	}

	// Wipe certificates from secure boot databases and reinitialize the
	// secure boot databases with the Incus certificates.
	toBeCleanedSecureBootDatabases := []string{"KEK", "DB", "DBX"}
	for _, secureBootDB := range secureBootDatabases {
		dbName := strings.ToUpper(secureBootDB.Name)
		if !slices.Contains(toBeCleanedSecureBootDatabases, dbName) {
			continue
		}

		certs, err := secureBootDB.Certificates()
		if err != nil {
			return fmt.Errorf("Failed to get secure boot database certificates: %w", err)
		}

		for _, cert := range certs {
			resp, err := client.Delete(cert.ODataID)
			if err != nil {
				slog.WarnContext(ctx, "Failed to delete secure boot certificate", slog.String("odata_id", cert.ODataID), logger.Err(err))
				continue
			}

			_ = resp.Body.Close()
		}

		for _, cert := range r.secureBootCertificates[dbName] {
			resp, err := client.Post(secureBootDB.ODataID, cert)
			if err != nil {
				return fmt.Errorf("Failed to add certificate to secure boot DB %q: %w", secureBootDB.ODataID, err)
			}

			_ = resp.Body.Close()

			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
				return fmt.Errorf("Unexpected status %d when adding certificate to secure boot DB %q", resp.StatusCode, secureBootDB.ODataID)
			}
		}
	}

	return nil
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

	actionInfo, err := system.ResetActionInfo()
	if err != nil {
		return nil, fmt.Errorf("Failed to get BMC reset action info: %w", err)
	}

	allowedResetTypes, err := actionInfo.GetParamValues("ResetType", schemas.StringParameterTypes)
	if err != nil {
		return nil, fmt.Errorf("Failed to get supported reset types from BMC: %w", err)
	}

	if !slices.Contains(allowedResetTypes, string(resetType)) {
		return nil, fmt.Errorf("Reset type %q is not supported by the BMC, supported types are: %v", resetType, allowedResetTypes)
	}

	taskMonitor, err := system.Reset(resetType)
	if err != nil {
		return nil, fmt.Errorf("Failed to perform BMC reset operation: %w", err)
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

	timestampFormats := []string{time.RFC3339, time.RFC3339Nano}

	bmcLogEvents := make([]api.BMCLogEvent, 0, len(entries))
	for _, entry := range entries {
		bmcLogEvents = append(bmcLogEvents, api.BMCLogEvent{
			EntryCode: string(entry.EntryCode),
			EntryType: string(entry.EntryType),
			Message:   entry.Message,
			Severity:  string(entry.Severity),
			Timestamp: parseTimestamp(timestampFormats, entry.Created, entry.EventTimestamp),
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

// parseTimestamp returns the first timestamp, which successfully parses for one
// of the provided formats. If none of the timestamps could be parsed with any
// of the formats, the zero time.Time is returned.
func parseTimestamp(formats []string, timestamps ...string) time.Time {
	for _, timestamp := range timestamps {
		for _, format := range formats {
			t, err := time.Parse(format, timestamp)
			if err == nil {
				return t
			}
		}
	}

	return time.Time{}
}
