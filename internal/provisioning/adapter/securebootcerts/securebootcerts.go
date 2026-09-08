// Package securebootcerts provides the catalog of well known UEFI
// certificates, that is shipped with Operations Center.
package securebootcerts

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"maps"
	"slices"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

//go:embed certs/*.pem
var builtinCertificates embed.FS

type entry struct {
	// fingerprint is the lower case hex encoded SHA256 fingerprint of the DER
	// encoding of the certificate.
	fingerprint string
	filename    string
	description string
}

// catalogDBCertificates are the well known certificates of the "db" key
// database, Operations Center ships the material of, so an entry, a BIOS profile
// allow lists by fingerprint alone, can be enrolled again after the key
// databases of a server have been wiped.
var catalogDBCertificates = []entry{
	{
		fingerprint: "48e99b991f57fc52f76149599bff0a58c47154229b9f8d603ac40d3500248507",
		filename:    "ms-corp-uefi-ca-2011.pem",
		description: "Microsoft Corporation UEFI CA 2011",
	},
	{
		fingerprint: "f6124e34125bee3fe6d79a574eaa7b91c0e7bd9d929c1a321178efd611dad901",
		filename:    "ms-uefi-ca-2023.pem",
		description: "Microsoft UEFI CA 2023",
	},
	{
		fingerprint: "e5be3e64c6e66a281457ecdece0d6d0787577aad2a3a0144262c10c14ba8d8f1",
		filename:    "ms-option-rom-uefi-ca-2023.pem",
		description: "Microsoft Option ROM UEFI CA 2023",
	},
	{
		fingerprint: "076f1fea90ac29155ebf77c17682f75f1fdd1be196da302dc8461e350a9ae330",
		filename:    "windows-uefi-ca-2023.pem",
		description: "Windows UEFI CA 2023",
	},
	{
		fingerprint: "89942cabd60392f5817bbde1ab7be34483ecb3ef2d282737dd266b6b30dfcce1",
		filename:    "lenovo-supplier-executable-ca-2017.pem",
		description: "Lenovo Supplier Executable CA 2017",
	},
	{
		fingerprint: "c3bf425c676ffab6158aad94607bb0c678cf9bb927c617c2e1312e9f58eb690f",
		filename:    "lenovo-uefi-db.pem",
		description: "Lenovo UEFI DB",
	},
}

// Catalog serves the well known UEFI certificates from an immutable set of
// PEM documents.
type Catalog struct {
	certificates map[string]string
	descriptions map[string]string
}

var _ provisioning.SecureBootCertificateCatalogPort = Catalog{}

// New returns the catalog of UEFI certificates shipped with Operations Center.
func New() (Catalog, error) {
	return newFromFS(builtinCertificates, "certs", catalogDBCertificates)
}

func newFromFS(fsys embed.FS, dir string, entries []entry) (Catalog, error) {
	catalog := Catalog{
		certificates: make(map[string]string, len(entries)),
		descriptions: make(map[string]string, len(entries)),
	}

	for _, entry := range entries {
		path := dir + "/" + entry.filename

		body, err := fsys.ReadFile(path)
		if err != nil {
			return Catalog{}, fmt.Errorf("Failed to read UEFI certificate from %q: %w", path, err)
		}

		fingerprint, err := certificateFingerprint(path, body)
		if err != nil {
			return Catalog{}, err
		}

		if fingerprint != entry.fingerprint {
			return Catalog{}, fmt.Errorf("UEFI certificate %q has the fingerprint %q instead of %q", path, fingerprint, entry.fingerprint)
		}

		_, duplicate := catalog.certificates[fingerprint]
		if duplicate {
			return Catalog{}, fmt.Errorf("Duplicate UEFI certificate %q in the catalog", fingerprint)
		}

		catalog.certificates[fingerprint] = string(body)
		catalog.descriptions[fingerprint] = entry.description
	}

	return catalog, nil
}

// certificateFingerprint returns the lower case hex encoded SHA256 fingerprint
// of the DER encoding of a PEM encoded certificate.
//
// The DER is hashed as it is, rather than being parsed first: some vendors,
// Lenovo among them, ship certificates, that x509.ParseCertificate rejects, for
// example for carrying an extension twice, while the firmware enrolls and trusts
// them just fine.
func certificateFingerprint(name string, pemCertificate []byte) (string, error) {
	block, _ := pem.Decode(pemCertificate)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("UEFI certificate %q does not contain a PEM encoded certificate", name)
	}

	sum := sha256.Sum256(block.Bytes)

	return hex.EncodeToString(sum[:]), nil
}

func (c Catalog) CertificatesByFingerprint(fingerprints []string) ([]string, []string) {
	var (
		certificates []string
		unknown      []string
	)

	for _, fingerprint := range fingerprints {
		certificate, ok := c.certificates[fingerprint]
		if !ok {
			unknown = append(unknown, fingerprint)
			continue
		}

		certificates = append(certificates, certificate)
	}

	return certificates, unknown
}

func (c Catalog) Describe(fingerprint string) string {
	description, ok := c.descriptions[fingerprint]
	if !ok {
		return fingerprint
	}

	return description
}

// Fingerprints returns the fingerprints of every certificate of the catalog,
// sorted, which lets a test tell the catalog and the allow lists apart.
func (c Catalog) Fingerprints() []string {
	return slices.Sorted(maps.Keys(c.certificates))
}
