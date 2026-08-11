package redfish

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/logger"
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

func (r redfish) ApplySecureBootCertificates(ctx context.Context, server provisioning.Server) error {
	if r.env == nil {
		return fmt.Errorf("Applying the secure boot certificates is not supported, no source for the certificates is configured: %w", domain.ErrOperationNotPermitted)
	}

	incusOSCertificates, err := r.env.GetSecureBootCertificates(ctx)
	if err != nil {
		return fmt.Errorf("Failed to get secure boot certificates from IncusOS: %w", err)
	}

	certificates, err := secureBootCertificatesByDatabase(incusOSCertificates)
	if err != nil {
		return err
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
		return fmt.Errorf("Failed to get secure boot information: %w", wrapRedfishError(err))
	}

	if secureBoot == nil {
		return fmt.Errorf("Applying the secure boot certificates is not supported, the BMC does not expose secure boot for system %q: %w", system.ODataID, domain.ErrOperationNotPermitted)
	}

	secureBootDatabases, err := secureBoot.SecureBootDatabases()
	if err != nil {
		return fmt.Errorf("Failed to get secure boot databases: %w", wrapRedfishError(err))
	}

	databases := secureBootDatabasesByName(secureBootDatabases)
	if len(databases) == 0 {
		return fmt.Errorf("Applying the secure boot certificates is not supported, the BMC provides %s for system %q: %w", describeSecureBootDatabases(secureBootDatabases), system.ODataID, domain.ErrOperationNotPermitted)
	}

	for _, dbName := range secureBootDatabaseNames {
		secureBootDB, ok := databases[dbName]
		if !ok {
			continue
		}

		err := wipeSecureBootDatabase(ctx, client, secureBootDB)
		if err != nil {
			return err
		}

		err = fillSecureBootDatabase(client, secureBootDB, certificates[dbName])
		if err != nil {
			return err
		}
	}

	return nil
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

var secureBootDBSignaturesAllowList = map[string][]string{}

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

// wipeSecureBootDatabase removes everything currently enrolled in a key
// database except the entries from the allow list.
func wipeSecureBootDatabase(ctx context.Context, client *gofish.APIClient, secureBootDB *schemas.SecureBootDatabase) error {
	secureBootDBID := secureBootDatabaseName(secureBootDB)

	signatures, err := secureBootDB.Signatures()
	if err != nil {
		return fmt.Errorf("Failed to get secure boot database signatures of %q: %w", secureBootDB.ODataID, wrapRedfishError(err))
	}

	for _, signature := range signatures {
		if slices.Contains(secureBootDBSignaturesAllowList[secureBootDBID], signature.SignatureString) {
			continue
		}

		err := deleteSecureBootEntry(client, signature.ODataID)
		if err != nil {
			return err
		}
	}

	certs, err := secureBootDB.Certificates()
	if err != nil {
		return fmt.Errorf("Failed to get secure boot database certificates of %q: %w", secureBootDB.ODataID, wrapRedfishError(err))
	}

	for _, cert := range certs {
		fingerprint, err := secureBootCertificateFingerprint(cert)
		if err != nil {
			slog.WarnContext(ctx, "Failed to calculate fingerprint of secure boot database certificate, removing it", logger.Err(err), slog.String("certificate", cert.ODataID), slog.String("certificate_subject", cert.Subject.CommonName))
		}

		if fingerprint != "" && slices.Contains(secureBootDBCertificateFingerprintAllowList[secureBootDBID], fingerprint) {
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
	block, _ := pem.Decode([]byte(cert.CertificateString))
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("Secure boot database certificate %q does not contain a PEM encoded certificate", cert.ODataID)
	}

	x509Certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("Failed to parse secure boot database certificate %q: %w", cert.ODataID, err)
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

// fillSecureBootDatabase enrolls the given PEM certificates into a key
// database.
func fillSecureBootDatabase(client *gofish.APIClient, secureBootDB *schemas.SecureBootDatabase, pemCertificates []string) error {
	if len(pemCertificates) == 0 {
		return nil
	}

	certificatesURI, err := secureBootDatabaseCertificatesURI(client, secureBootDB.ODataID)
	if err != nil {
		return err
	}

	for _, pemCertificate := range pemCertificates {
		payload := struct {
			CertificateString string                  `json:"CertificateString"`
			CertificateType   schemas.CertificateType `json:"CertificateType"`
		}{
			CertificateString: pemCertificate,
			CertificateType:   schemas.PEMCertificateType,
		}

		resp, err := client.Post(certificatesURI, payload)
		if err != nil {
			return fmt.Errorf("Failed to add certificate to secure boot DB %q: %w", secureBootDB.ODataID, redfishRequestError(err, nil, http.MethodPost, certificatesURI, payload))
		}

		_ = resp.Body.Close()
	}

	return nil
}

// secureBootDatabaseCertificatesURI resolves the certificate collection a new
// certificate has to be posted to.
// TODO: replace when https://github.com/stmcginnis/gofish/issues/559 is resolved.
func secureBootDatabaseCertificatesURI(client *gofish.APIClient, dbODataID string) (string, error) {
	var raw struct {
		Certificates schemas.Link `json:"Certificates"`
	}

	err := getJSON(client, dbODataID, &raw)
	if err != nil {
		return "", fmt.Errorf("Failed to get secure boot database %q: %w", dbODataID, wrapRedfishError(err))
	}

	if raw.Certificates.String() == "" {
		return "", fmt.Errorf("Secure boot database %q does not provide a certificate collection: %w", dbODataID, domain.ErrOperationNotPermitted)
	}

	return raw.Certificates.String(), nil
}

// getJSON fetches uri and decodes the raw response body into target. It exists
// to reach the parts of a resource gofish does not expose.
func getJSON(client *gofish.APIClient, uri string, target any) error {
	resp, err := client.Get(uri)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return json.Unmarshal(body, target)
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
