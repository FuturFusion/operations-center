package redfish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/shared/api"
)

const (
	secureBootDatabaseKEK = api.SecureBootDatabaseKEK
	secureBootDatabaseDB  = api.SecureBootDatabaseDB
	secureBootDatabaseDBX = api.SecureBootDatabaseDBX
)

// secureBootDatabaseNames are the UEFI secure boot key databases which are
// wiped and reinitialized with the certificates provided by IncusOS, in the
// order they are processed.
var secureBootDatabaseNames = []string{
	secureBootDatabaseKEK,
	secureBootDatabaseDB,
	secureBootDatabaseDBX,
}

func (r redfish) ApplySecureBootCertificates(ctx context.Context, server provisioning.Server, secureBoot api.BIOSSecureBoot) (bool, error) {
	if r.env == nil {
		return false, domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Applying the secure boot certificates is not supported, no source for the certificates is configured")
	}

	incusOSCertificates, err := r.env.GetSecureBootCertificates(ctx)
	if err != nil {
		return false, fmt.Errorf("Failed to get secure boot certificates from IncusOS: %w", err)
	}

	certificates, err := secureBootCertificatesByDatabase(incusOSCertificates)
	if err != nil {
		return false, err
	}

	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return false, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	system, err := getFirstSystem(client)
	if err != nil {
		return false, fmt.Errorf("Failed get BMC system: %w", err)
	}

	systemSecureBoot, err := system.SecureBoot()
	if err != nil {
		return false, fmt.Errorf("Failed to get secure boot information: %w", wrapRedfishError(err))
	}

	if systemSecureBoot == nil {
		return false, domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Applying the secure boot certificates is not supported, the BMC does not expose secure boot for system %q", system.ODataID).
			WithDetail("system", system.ODataID)
	}

	secureBootDatabases, err := systemSecureBoot.SecureBootDatabases()
	if err != nil {
		return false, fmt.Errorf("Failed to get secure boot databases: %w", wrapRedfishError(err))
	}

	databases := secureBootDatabasesByName(secureBootDatabases)
	if len(databases) == 0 {
		return false, domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Applying the secure boot certificates is not supported, the BMC provides %s for system %q", describeSecureBootDatabases(secureBootDatabases), system.ODataID).
			WithDetail("system", system.ODataID)
	}

	enrolled := false

	for _, dbName := range secureBootDatabaseNames {
		secureBootDB, ok := databases[dbName]
		if !ok {
			continue
		}

		state, err := readSecureBootDatabase(secureBootDB)
		if err != nil {
			return enrolled, err
		}

		allowList := secureBootAllowList(dbName, secureBoot)

		if secureBootDatabaseApplied(state, allowList, certificates[dbName]) {
			slog.InfoContext(ctx, "Secure boot database holds the certificates of IncusOS already, leaving it untouched", slog.String("database", secureBootDB.ODataID))
			continue
		}

		enrolled = true

		err = wipeSecureBootDatabase(ctx, client, state, allowList)
		if err != nil {
			return enrolled, err
		}

		err = fillSecureBootDatabase(secureBootDB, certificates[dbName])
		if err != nil {
			return enrolled, err
		}
	}

	return enrolled, nil
}

// secureBootResetKeysTypes are the resets, that put a server into the secure
// boot setup mode, in the order they are attempted. Wiping every key database
// is more reliable, so it is attempted first. Deleting only the platform key
// should per specification also be enough.
var secureBootResetKeysTypes = []schemas.ResetKeysType{
	schemas.DeleteAllKeysResetKeysType,
	schemas.DeletePKResetKeysType,
}

func (r redfish) ResetSecureBootKeys(ctx context.Context, server provisioning.Server) (bool, *provisioning.BMCTaskMonitor, error) {
	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return false, nil, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	system, err := getFirstSystem(client)
	if err != nil {
		return false, nil, fmt.Errorf("Failed get BMC system: %w", err)
	}

	systemSecureBoot, err := system.SecureBoot()
	if err != nil {
		return false, nil, fmt.Errorf("Failed to get secure boot information: %w", wrapRedfishError(err))
	}

	if systemSecureBoot == nil {
		return false, nil, domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Resetting the secure boot keys is not supported, the BMC does not expose secure boot for system %q", system.ODataID).
			WithDetail("system", system.ODataID)
	}

	if systemSecureBoot.SecureBootMode == schemas.SetupModeSecureBootModeType {
		slog.InfoContext(ctx, "Server is in secure boot setup mode already, leaving its key databases untouched", slog.String("secure_boot", systemSecureBoot.ODataID))

		return false, nil, nil
	}

	if !secureBootSupportsResetKeys(systemSecureBoot) {
		return false, nil, domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Resetting the secure boot keys is not supported, the BMC does not provide the reset keys action for system %q", system.ODataID).
			WithDetail("system", system.ODataID)
	}

	var errs []error

	for _, resetKeysType := range secureBootResetKeysTypes {
		taskMonitor, err := systemSecureBoot.ResetKeys(resetKeysType)
		if err == nil {
			slog.InfoContext(ctx, "Secure boot keys reset", slog.String("secure_boot", systemSecureBoot.ODataID), slog.String("reset_keys_type", string(resetKeysType)))

			if taskMonitor == nil {
				return true, nil, nil
			}

			return true, &provisioning.BMCTaskMonitor{
				URI: taskMonitor.TaskMonitor,
			}, nil
		}

		errs = append(errs, fmt.Errorf("Failed to reset the secure boot keys of %q with %q: %w", systemSecureBoot.ODataID, resetKeysType, wrapRedfishError(err)))
	}

	return false, nil, errors.Join(errs...)
}

// secureBootSupportsResetKeys reports, whether the BMC published the reset keys
// action.
func secureBootSupportsResetKeys(systemSecureBoot *schemas.SecureBoot) bool {
	var actions struct {
		Actions struct {
			ResetKeys struct {
				Target string `json:"target"`
			} `json:"#SecureBoot.ResetKeys"`
		} `json:"Actions"`
	}

	err := json.Unmarshal(systemSecureBoot.RawData, &actions)
	if err != nil {
		// The BMC answered with something, that can not be inspected, so let the
		// reset itself report what is wrong.
		return true
	}

	return actions.Actions.ResetKeys.Target != ""
}

// secureBootCertificatesByDatabase groups the certificates IncusOS provides by
// the key database they belong into.
func secureBootCertificatesByDatabase(incusOSCertificates incusosapi.InternalSecureBootCertificates) (map[string][]string, error) {
	certificates := map[string][]string{
		secureBootDatabaseKEK: nonEmptyCertificates(incusOSCertificates.KEK),
		secureBootDatabaseDB:  nonEmptyCertificates(incusOSCertificates.DB),
		secureBootDatabaseDBX: nonEmptyCertificates(incusOSCertificates.DBX),
	}

	total := 0
	for _, certs := range certificates {
		total += len(certs)
	}

	if total == 0 {
		return nil, domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Applying the secure boot certificates is not possible, IncusOS did not provide any certificates")
	}

	return certificates, nil
}

func nonEmptyCertificates(pemCertificates []string) []string {
	certificates := make([]string, 0, len(pemCertificates))

	for _, pemCertificate := range pemCertificates {
		if pemCertificate == "" {
			continue
		}

		certificates = append(certificates, pemCertificate)
	}

	return certificates
}

// secureBootDatabasesByName picks the key databases to be reinitialized out of
// the databases published by the BMC, keyed by their UEFI name.
func secureBootDatabasesByName(secureBootDatabases []*schemas.SecureBootDatabase) map[string]*schemas.SecureBootDatabase {
	databases := map[string]*schemas.SecureBootDatabase{}

	for _, secureBootDB := range secureBootDatabases {
		dbName := secureBootDatabaseName(secureBootDB)
		if dbName == "" {
			continue
		}

		databases[dbName] = secureBootDB
	}

	return databases
}

// secureBootDatabaseName reports which UEFI key database a resource represents,
// or an empty string for a database which is not to be touched.
func secureBootDatabaseName(secureBootDB *schemas.SecureBootDatabase) string {
	for _, candidate := range []string{secureBootDB.DatabaseID, secureBootDB.ID, secureBootDB.Name} {
		name := strings.TrimSpace(candidate)
		if slices.Contains(secureBootDatabaseNames, name) {
			return name
		}
	}

	return ""
}

// secureBootAllowListEntries names the entries of a single key database, which
// survive its reinitialization.
type secureBootAllowListEntries struct {
	certificates map[string]struct{}
	signatures   map[string]struct{}
}

func secureBootAllowList(dbName string, secureBoot api.BIOSSecureBoot) secureBootAllowListEntries {
	database := secureBootProfileDatabase(dbName, secureBoot)

	return secureBootAllowListEntries{
		certificates: secureBootAllowSet(database.Certificates, strings.ToLower),
		signatures:   secureBootAllowSet(database.Signatures, nil),
	}
}

func secureBootProfileDatabase(dbName string, secureBoot api.BIOSSecureBoot) api.BIOSSecureBootDatabase {
	switch dbName {
	case secureBootDatabaseKEK:
		return secureBoot.KEK

	case secureBootDatabaseDB:
		return secureBoot.DB

	case secureBootDatabaseDBX:
		return secureBoot.DBX
	}

	return api.BIOSSecureBootDatabase{}
}

// secureBootAllowSet collects the entries, that survive the wipe of a key
// database, from what the BIOS profiles of a server resolved to.
func secureBootAllowSet(overrides map[string]bool, normalize func(string) string) map[string]struct{} {
	if normalize == nil {
		normalize = func(entry string) string { return entry }
	}

	allowed := make(map[string]struct{}, len(overrides))

	for entry, keep := range overrides {
		entry = normalize(strings.TrimSpace(entry))

		if !keep {
			continue
		}

		allowed[entry] = struct{}{}
	}

	return allowed
}

// secureBootDatabaseState is, what the BMC reports about a single key database.
// Both collections are read before anything is deleted, so the applied check and
// the wipe work off the very same view.
type secureBootDatabaseState struct {
	signatures   []*schemas.Signature
	certificates []*schemas.Certificate
}

func readSecureBootDatabase(secureBootDB *schemas.SecureBootDatabase) (secureBootDatabaseState, error) {
	signatures, err := secureBootDB.Signatures()
	if err != nil {
		return secureBootDatabaseState{}, fmt.Errorf("Failed to get secure boot database signatures of %q: %w", secureBootDB.ODataID, wrapRedfishError(err))
	}

	certs, err := secureBootDB.Certificates()
	if err != nil {
		return secureBootDatabaseState{}, fmt.Errorf("Failed to get secure boot database certificates of %q: %w", secureBootDB.ODataID, wrapRedfishError(err))
	}

	return secureBootDatabaseState{
		signatures:   signatures,
		certificates: certs,
	}, nil
}

// secureBootDatabaseApplied reports, whether reinitializing a key database would
// change nothing.
func secureBootDatabaseApplied(state secureBootDatabaseState, allowList secureBootAllowListEntries, pemCertificates []string) bool {
	for _, signature := range state.signatures {
		_, allowed := allowList.signatures[signature.SignatureString]
		if !allowed {
			return false
		}
	}

	desired := make(map[string]struct{}, len(pemCertificates))

	for _, pemCertificate := range pemCertificates {
		// A certificate, that can not be parsed, can not be looked for either,
		// so the database has to be reinitialized. The enrollment then posts it
		// verbatim and lets the BMC reject it.
		fingerprint, err := pemCertificateFingerprint("of IncusOS", pemCertificate)
		if err != nil {
			return false
		}

		desired[fingerprint] = struct{}{}
	}

	enrolled := make(map[string]struct{}, len(state.certificates))

	for _, cert := range state.certificates {
		fingerprint, err := secureBootCertificateFingerprint(cert)
		if err != nil {
			return false
		}

		// A fingerprint, that is enrolled twice, is collapsed into a single
		// entry by the reinitialization, which is a change.
		_, duplicate := enrolled[fingerprint]
		if duplicate {
			return false
		}

		enrolled[fingerprint] = struct{}{}

		_, wanted := desired[fingerprint]
		_, allowed := allowList.certificates[fingerprint]

		if !wanted && !allowed {
			return false
		}
	}

	for fingerprint := range desired {
		_, ok := enrolled[fingerprint]
		if !ok {
			return false
		}
	}

	return true
}

// wipeSecureBootDatabase removes everything currently enrolled in a key
// database except the entries from the allow list.
func wipeSecureBootDatabase(ctx context.Context, client *gofish.APIClient, state secureBootDatabaseState, allowList secureBootAllowListEntries) error {
	for _, signature := range state.signatures {
		_, allowed := allowList.signatures[signature.SignatureString]
		if allowed {
			continue
		}

		err := deleteSecureBootEntry(client, signature.ODataID)
		if err != nil {
			return err
		}
	}

	for _, cert := range state.certificates {
		fingerprint, err := secureBootCertificateFingerprint(cert)
		if err != nil {
			slog.WarnContext(ctx, "Failed to calculate fingerprint of secure boot database certificate, removing it", logger.Err(err), slog.String("certificate", cert.ODataID), slog.String("certificate_subject", cert.Subject.CommonName))
		}

		_, allowed := allowList.certificates[fingerprint]
		if fingerprint != "" && allowed {
			continue
		}

		err = deleteSecureBootEntry(client, cert.ODataID)
		if err != nil {
			return err
		}
	}

	return nil
}

func secureBootCertificateFingerprint(cert *schemas.Certificate) (string, error) {
	return pemCertificateFingerprint(cert.ODataID, cert.CertificateString)
}

// pemCertificateFingerprint returns the lower case hex encoded SHA256
// fingerprint of the DER encoding of a PEM encoded certificate.
//
// The DER is hashed as it is, rather than being parsed first: some vendors,
// Lenovo among them, enroll certificates, that x509.ParseCertificate rejects,
// for example for carrying an extension twice. Parsing them would leave them
// without a fingerprint to match an allow list against, which has them wiped
// from the key database, and with them the trust in the option ROMs they sign.
func pemCertificateFingerprint(name string, pemCertificate string) (string, error) {
	block, _ := pem.Decode([]byte(pemCertificate))
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("Secure boot database certificate %q does not contain a PEM encoded certificate", name)
	}

	sum := sha256.Sum256(block.Bytes)

	return hex.EncodeToString(sum[:]), nil
}

func deleteSecureBootEntry(client *gofish.APIClient, odataID string) error {
	resp, err := client.Delete(odataID)
	if err != nil {
		return fmt.Errorf("Failed to delete secure boot entry %q: %w", odataID, wrapRedfishError(err))
	}

	_ = resp.Body.Close()

	return nil
}

func fillSecureBootDatabase(secureBootDB *schemas.SecureBootDatabase, pemCertificates []string) error {
	for _, pemCertificate := range pemCertificates {
		_, err := secureBootDB.AddCertificate(pemCertificate, schemas.PEMCertificateType, "")
		if err != nil {
			return fmt.Errorf("Failed to add certificate to secure boot DB %q: %w", secureBootDB.ODataID, wrapRedfishError(err))
		}
	}

	return nil
}

// describeSecureBootDatabases names the secure boot databases the BMC published,
// so that an unusable set of databases can be told apart from none at all.
func describeSecureBootDatabases(secureBootDatabases []*schemas.SecureBootDatabase) string {
	if len(secureBootDatabases) == 0 {
		return "any secure boot databases"
	}

	names := make([]string, 0, len(secureBootDatabases))
	for _, secureBootDB := range secureBootDatabases {
		names = append(names, secureBootDB.ODataID)
	}

	return fmt.Sprintf("a usable secure boot database among %v", names)
}
