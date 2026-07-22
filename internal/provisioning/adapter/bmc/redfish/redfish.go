package redfish

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/shared/api"
)

type redfish struct{}

var _ provisioning.BMCServerClientPort = redfish{}

func New() redfish {
	return redfish{}
}

func (r redfish) getClient(ctx context.Context, server provisioning.Server) (_ *gofish.APIClient, logout func(), _ error) {
	if transaction.IsActive(ctx) {
		slog.WarnContext(ctx, "Redfish API call inside of a transaction", logger.AddStacktrace())
	}

	c, err := gofish.Connect(gofish.ClientConfig{
		Endpoint: server.BMCEndpoint,
		Username: server.BMCUsername,
		Password: server.BMCPassword,
		// Insecure: true,
		// BasicAuth: true,
		// DumpWriter: os.Stdout,
	})
	if err != nil {
		return nil, nil, err
	}

	return c, c.Logout, nil
}

func (r redfish) GetServerDetails(ctx context.Context, server provisioning.Server) (api.BMCServerDetails, error) {
	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return api.BMCServerDetails{}, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCEndpoint, err)
	}

	defer logout()

	systems, err := client.Service.Systems()
	if err != nil {
		return api.BMCServerDetails{}, fmt.Errorf("Failed to get BMC systems on %q: %w", server.BMCEndpoint, err)
	}

	if len(systems) == 0 {
		return api.BMCServerDetails{}, fmt.Errorf("No BMC systems found on %q", server.BMCEndpoint)
	}

	system := systems[0]

	return api.BMCServerDetails{
		SystemUUID: system.ID,
	}, nil
}

func (r redfish) Start(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
	resetType := schemas.OnResetType
	if force {
		resetType = schemas.ForceOnResetType
	}

	return r.performReset(ctx, server, resetType)
}

func (r redfish) Stop(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
	resetType := schemas.GracefulShutdownResetType
	if force {
		resetType = schemas.ForceOffResetType
	}

	return r.performReset(ctx, server, resetType)
}

func (r redfish) Restart(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
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
		return fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCEndpoint, err)
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

func (r redfish) SetupBIOS(ctx context.Context, server provisioning.Server) (*provisioning.BMCTaskMonitor, error) {
	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCEndpoint, err)
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

	// TODO: unfortunately, UpdateBiosAttributesApplyAt does not return the
	// location of the task created for this operation, therfore there is
	// currently no good way to wait for the task to complete.
	// See discussion in: https://github.com/stmcginnis/gofish/issues/472#issuecomment-5045603910
	err = bios.UpdateBiosAttributesApplyAt(schemas.SettingsAttributes{
		"NumaNodesPerSocket": "4",
		"SecureBoot":         "Enabled",
		"SecureBootMode":     "UserMode",
		"SecureBootPolicy":   "Custom",
		"TpmSecurity":        "On",
	}, schemas.OnResetSettingsApplyTime)
	if err != nil {
		return nil, fmt.Errorf("Failed to apply bios attributes: %w", err)
	}

	return nil, nil
}

func (r redfish) SetupSecureBootCertificates(ctx context.Context, server provisioning.Server) error {
	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCEndpoint, err)
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

		uploadCerts := []schemas.Certificate{}

		switch dbName {
		case "KEK":
			uploadCerts = append(uploadCerts, schemas.Certificate{
				// FIXME: add the correct certificates here
				CertificateString: "certPEM",
				CertificateType:   schemas.PEMCertificateType,
			})

		case "DB":
			uploadCerts = append(uploadCerts, schemas.Certificate{
				// FIXME: add the correct certificates here
				CertificateString: "certPEM1",
				CertificateType:   schemas.PEMCertificateType,
			})
			uploadCerts = append(uploadCerts, schemas.Certificate{
				// FIXME: add the correct certificates here
				CertificateString: "certPEM2",
				CertificateType:   schemas.PEMCertificateType,
			})
		}

		for _, cert := range uploadCerts {
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
		return nil, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCEndpoint, err)
	}

	defer logout()

	system, err := getFirstSystem(client)
	if err != nil {
		return nil, fmt.Errorf("Failed get BMC system: %w", err)
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

func getFirstSystem(client *gofish.APIClient) (*schemas.ComputerSystem, error) {
	systems, err := client.Service.Systems()
	if err != nil {
		return nil, fmt.Errorf("Failed to get BMC systems: %w", err)
	}

	if len(systems) == 0 {
		return nil, fmt.Errorf("No BMC systems found")
	}

	sort.Slice(systems, func(i, j int) bool { return systems[i].ID < systems[j].ID })

	return systems[0], nil
}
