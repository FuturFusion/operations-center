package redfish

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/stmcginnis/gofish/schemas"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
)

var testdataCertificates = map[string]string{
	"microsoft-corporation-uefi-ca-2011.pem": "48e99b991f57fc52f76149599bff0a58c47154229b9f8d603ac40d3500248507",
	"microsoft-uefi-ca-2023.pem":             "f6124e34125bee3fe6d79a574eaa7b91c0e7bd9d929c1a321178efd611dad901",
	"microsoft-option-rom-uefi-ca-2023.pem":  "e5be3e64c6e66a281457ecdece0d6d0787577aad2a3a0144262c10c14ba8d8f1",
}

func TestSecureBootCertificateFingerprint(t *testing.T) {
	tests := []struct {
		name              string
		certificateString string

		want      string
		assertErr require.ErrorAssertionFunc
	}{
		{
			name:              "success",
			certificateString: readTestdataCertificate(t, "microsoft-corporation-uefi-ca-2011.pem"),

			want:      "48e99b991f57fc52f76149599bff0a58c47154229b9f8d603ac40d3500248507",
			assertErr: require.NoError,
		},
		{
			name: "success - a chain is identified by its leaf certificate",
			certificateString: readTestdataCertificate(t, "microsoft-uefi-ca-2023.pem") +
				readTestdataCertificate(t, "microsoft-option-rom-uefi-ca-2023.pem"),

			want:      "f6124e34125bee3fe6d79a574eaa7b91c0e7bd9d929c1a321178efd611dad901",
			assertErr: require.NoError,
		},
		{
			name:              "error - no certificate reported by the BMC",
			certificateString: "",

			assertErr: errassert.Contains("does not contain a PEM encoded certificate"),
		},
		{
			name:              "error - not PEM encoded",
			certificateString: "not a PEM encoded certificate",

			assertErr: errassert.Contains("does not contain a PEM encoded certificate"),
		},
		{
			name:              "error - PEM block is not a certificate",
			certificateString: string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("public key")})),

			assertErr: errassert.Contains("does not contain a PEM encoded certificate"),
		},
		{
			name:              "error - PEM block does not contain a valid certificate",
			certificateString: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("not a certificate")})),

			assertErr: errassert.Contains("Failed to parse secure boot database certificate"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cert := &schemas.Certificate{
				CertificateString: tc.certificateString,
			}

			cert.ODataID = "/redfish/v1/Systems/1/SecureBoot/SecureBootDatabases/db/Certificates/1"

			got, err := secureBootCertificateFingerprint(cert)

			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestSecureBootDBCertificateFingerprintAllowList(t *testing.T) {
	require.Equal(t, []string{secureBootDatabaseDB}, keys(secureBootDBCertificateFingerprintAllowList))

	wantFingerprints := make([]string, 0, len(testdataCertificates))

	for file, wantFingerprint := range testdataCertificates {
		block, _ := pem.Decode([]byte(readTestdataCertificate(t, file)))
		require.NotNil(t, block, file)

		certificate, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err, file)

		fingerprint, err := secureBootCertificateFingerprint(&schemas.Certificate{
			CertificateString: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})),
		})
		require.NoError(t, err, file)
		require.Equal(t, wantFingerprint, fingerprint, file)

		wantFingerprints = append(wantFingerprints, wantFingerprint)
	}

	require.ElementsMatch(t, wantFingerprints, secureBootDBCertificateFingerprintAllowList[secureBootDatabaseDB])
}

func TestSecureBootCertificatesByDatabase(t *testing.T) {
	tests := []struct {
		name                string
		incusOSCertificates incusosapi.InternalSecureBootCertificates

		want      map[string][]string
		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			incusOSCertificates: incusosapi.InternalSecureBootCertificates{
				PK:  "pkCert",
				KEK: []string{"kekCert"},
				DB:  []string{"dbCert1", "dbCert2"},
				DBX: []string{"dbxCert"},
			},

			want: map[string][]string{
				secureBootDatabaseKEK: {"kekCert"},
				secureBootDatabaseDB:  {"dbCert1", "dbCert2"},
				secureBootDatabaseDBX: {"dbxCert"},
			},
			assertErr: require.NoError,
		},
		{
			name: "success - empty certificates are dropped",
			incusOSCertificates: incusosapi.InternalSecureBootCertificates{
				KEK: []string{"", "kekCert", ""},
			},

			want: map[string][]string{
				secureBootDatabaseKEK: {"kekCert"},
				secureBootDatabaseDB:  {},
				secureBootDatabaseDBX: {},
			},
			assertErr: require.NoError,
		},
		{
			name: "error - IncusOS did not provide any certificates",
			// The PK is never enrolled, so it does not count towards the
			// certificates to be applied.
			incusOSCertificates: incusosapi.InternalSecureBootCertificates{
				PK:  "pkCert",
				KEK: []string{""},
			},

			assertErr: errassert.OperationNotPermittedError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := secureBootCertificatesByDatabase(tc.incusOSCertificates)

			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestSecureBootDatabaseName(t *testing.T) {
	tests := []struct {
		name         string
		secureBootDB *schemas.SecureBootDatabase

		want string
	}{
		{
			name:         "database ID",
			secureBootDB: newSecureBootDatabase("db", "Certificates", "UEFI Signature Database"),

			want: secureBootDatabaseDB,
		},
		{
			name:         "resource ID",
			secureBootDB: newSecureBootDatabase("", "KEK", "UEFI Key Exchange Key Database"),

			want: secureBootDatabaseKEK,
		},
		{
			name:         "resource name",
			secureBootDB: newSecureBootDatabase("", "1", "dbx"),

			want: secureBootDatabaseDBX,
		},
		{
			name:         "surrounding whitespace is ignored",
			secureBootDB: newSecureBootDatabase(" db ", "", ""),

			want: secureBootDatabaseDB,
		},
		{
			name: "database not to be touched",
			// The platform key is never wiped, losing it would take the server
			// out of user mode.
			secureBootDB: newSecureBootDatabase("PK", "PK", "UEFI Platform Key"),

			want: "",
		},
		{
			name:         "the match is case sensitive, DB is not db",
			secureBootDB: newSecureBootDatabase("DB", "DB", "DB"),

			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, secureBootDatabaseName(tc.secureBootDB))
		})
	}
}

func TestSecureBootDatabasesByName(t *testing.T) {
	kek := newSecureBootDatabase("KEK", "KEK", "UEFI Key Exchange Key Database")
	db := newSecureBootDatabase("db", "db", "UEFI Signature Database")
	pk := newSecureBootDatabase("PK", "PK", "UEFI Platform Key")

	got := secureBootDatabasesByName([]*schemas.SecureBootDatabase{kek, db, pk})

	require.Equal(t, map[string]*schemas.SecureBootDatabase{
		secureBootDatabaseKEK: kek,
		secureBootDatabaseDB:  db,
	}, got)
}

func TestDescribeSecureBootDatabases(t *testing.T) {
	tests := []struct {
		name                string
		secureBootDatabases []*schemas.SecureBootDatabase

		want string
	}{
		{
			name:                "no databases at all",
			secureBootDatabases: nil,

			want: "any secure boot databases",
		},
		{
			name: "only databases which are not to be touched",
			secureBootDatabases: []*schemas.SecureBootDatabase{
				newSecureBootDatabaseWithODataID("/redfish/v1/Systems/1/SecureBoot/SecureBootDatabases/PK"),
			},

			want: "a usable secure boot database among [/redfish/v1/Systems/1/SecureBoot/SecureBootDatabases/PK]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, describeSecureBootDatabases(tc.secureBootDatabases))
		})
	}
}

func newSecureBootDatabase(databaseID string, id string, name string) *schemas.SecureBootDatabase {
	secureBootDB := &schemas.SecureBootDatabase{
		DatabaseID: databaseID,
	}

	secureBootDB.ID = id
	secureBootDB.Name = name

	return secureBootDB
}

func newSecureBootDatabaseWithODataID(odataID string) *schemas.SecureBootDatabase {
	secureBootDB := &schemas.SecureBootDatabase{}
	secureBootDB.ODataID = odataID

	return secureBootDB
}

func readTestdataCertificate(t *testing.T, file string) string {
	t.Helper()

	pemCertificate, err := os.ReadFile(filepath.Join("testdata", file))
	require.NoError(t, err)

	return string(pemCertificate)
}

func keys[V any](m map[string]V) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}

	return names
}
