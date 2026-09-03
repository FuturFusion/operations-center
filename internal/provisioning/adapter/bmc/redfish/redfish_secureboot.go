package redfish

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
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
	secureBootDatabaseKEK = "KEK"
	secureBootDatabaseDB  = "db"
	secureBootDatabaseDBX = "dbx"
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
		return false, fmt.Errorf("Applying the secure boot certificates is not supported, no source for the certificates is configured: %w", domain.ErrOperationNotPermitted)
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
		return false, fmt.Errorf("Applying the secure boot certificates is not supported, the BMC does not expose secure boot for system %q: %w", system.ODataID, domain.ErrOperationNotPermitted)
	}

	secureBootDatabases, err := systemSecureBoot.SecureBootDatabases()
	if err != nil {
		return false, fmt.Errorf("Failed to get secure boot databases: %w", wrapRedfishError(err))
	}

	databases := secureBootDatabasesByName(secureBootDatabases)
	if len(databases) == 0 {
		return false, fmt.Errorf("Applying the secure boot certificates is not supported, the BMC provides %s for system %q: %w", describeSecureBootDatabases(secureBootDatabases), system.ODataID, domain.ErrOperationNotPermitted)
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
		return nil, fmt.Errorf("Applying the secure boot certificates is not possible, IncusOS did not provide any certificates: %w", domain.ErrOperationNotPermitted)
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

// secureBootDBCertificateFingerprintAllowList contains the lower case hex
// encoded SHA256 fingerprints of the DER encoding of the certificates, which
// are kept while wiping a key database. The fingerprint is calculated from the
// certificate itself.
var secureBootDBCertificateFingerprintAllowList = map[string][]string{
	secureBootDatabaseDB: {
		"48e99b991f57fc52f76149599bff0a58c47154229b9f8d603ac40d3500248507", // DB Microsoft Corporation UEFI CA 2011
		"f6124e34125bee3fe6d79a574eaa7b91c0e7bd9d929c1a321178efd611dad901", // DB Microsoft UEFI CA 2023
		"e5be3e64c6e66a281457ecdece0d6d0787577aad2a3a0144262c10c14ba8d8f1", // DB Microsoft Option ROM UEFI CA 2023
	},
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
		certificates: secureBootAllowSet(secureBootDBCertificateFingerprintAllowList[dbName], database.Certificates, strings.ToLower),
		signatures:   secureBootAllowSet(nil, database.Signatures, nil),
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

// secureBootAllowSet folds the overrides of the BIOS profiles into the built in
// defaults. An override of true adds an entry, one of false drops it, and an
// entry, no override names at all, keeps whatever the defaults say.
func secureBootAllowSet(defaults []string, overrides map[string]bool, normalize func(string) string) map[string]struct{} {
	if normalize == nil {
		normalize = func(entry string) string { return entry }
	}

	allowed := make(map[string]struct{}, len(defaults)+len(overrides))

	for _, entry := range defaults {
		allowed[normalize(entry)] = struct{}{}
	}

	for entry, keep := range overrides {
		entry = normalize(strings.TrimSpace(entry))

		if !keep {
			delete(allowed, entry)
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

func pemCertificateFingerprint(name string, pemCertificate string) (string, error) {
	block, _ := pem.Decode([]byte(pemCertificate))
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("Secure boot database certificate %q does not contain a PEM encoded certificate", name)
	}

	x509Certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("Failed to parse secure boot database certificate %q: %w", name, err)
	}

	sum := sha256.Sum256(x509Certificate.Raw)

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
