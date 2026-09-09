package securebootmedia

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

const (
	// espSize is the size of the EFI system partition of the enrollment media.
	// It holds the boot loader, which is about 128 KiB, plus a handful of key
	// database updates, so it is roomy by a wide margin.
	espSize = 3 * 1024 * 1024

	// espVolumeID is the FAT volume ID of the EFI system partition. It is fixed,
	// so that the very same certificates produce the very same image.
	espVolumeID = "0e4c3b2a"

	// espFileTime is the modification time every file of the EFI system
	// partition is given, for the same reason. FAT can not represent anything
	// before 1980, which is why it is not the Unix epoch.
	espFileTime = "1980-01-01T00:00:00Z"

	// authTimestamp is the timestamp of the generated key database updates. A
	// fixed timestamp keeps the image reproducible and lets a later, properly
	// signed update supersede what is enrolled here. It plays no role while the
	// server is in setup mode, where the firmware does not check the updates at
	// all.
	authTimestamp = "1970-01-01 00:00:00"

	// ownerGUID is the signature owner IncusOS uses for its own key database
	// entries, so that the entries enrolled here are indistinguishable from the
	// ones IncusOS enrolls itself.
	// FIXME: -- switch to the HypervisorOS UUID.
	ownerGUID = "433f8160-9ab6-4407-8e38-12d70e1d54e5"

	// signingKeyCommonName names the throw away key the key database updates are
	// signed with. The signature is never checked, since the enrollment only
	// happens while the server is in secure boot setup mode.
	signingKeyCommonName = "operations-center-secure-boot-enrollment"
)

// loaderConf is the boot loader configuration, that makes systemd-boot enroll
// the key databases found next to it without asking.
const loaderConf = `secure-boot-enroll force
timeout 3
editor no
`

// secureBootDatabases are the UEFI key databases written to the enrollment
// media, in the order systemd-boot enrolls them in. The platform key comes last,
// since enrolling it takes the server out of setup mode.
var secureBootDatabases = []string{
	api.SecureBootDatabaseDBX,
	api.SecureBootDatabaseDB,
	api.SecureBootDatabaseKEK,
	secureBootDatabasePK,
}

const secureBootDatabasePK = "PK"

// MediaID returns the ID addressing the enrollment media, that boots on
// architecture and enrolls the certificates, without generating anything.
//
// It is derived from what the media holds alone, so asking for the very same
// media twice addresses the very same image. The architecture is part of it,
// since the boot loader of a media differs by it.
func MediaID(architecture images.UpdateFileArchitecture, certificates provisioning.SecureBootCertificates) string {
	return provisioning.SeedImageFingerprintID(mediaFingerprint(architecture, certificates))
}

func mediaFingerprint(architecture images.UpdateFileArchitecture, certificates provisioning.SecureBootCertificates) string {
	sum := sha256.New()

	// The generator version is part of the fingerprint, so a change of the
	// layout does not keep serving what has been generated before. Writing to a
	// hash never fails.
	parts := make([]string, 0, 2+2*len(secureBootDatabases))
	parts = append(parts, strconv.Itoa(generatorVersion), string(architecture))

	for _, database := range secureBootDatabases {
		parts = append(parts, database)
		parts = append(parts, certificatesOf(certificates, database)...)
	}

	for _, part := range parts {
		_, _ = sum.Write([]byte(part + "\x00"))
	}

	return hex.EncodeToString(sum.Sum(nil))
}

// generatorVersion is bumped whenever the generated image changes, so that a
// cached image of an older version is not served anymore.
const generatorVersion = 1

func certificatesOf(certificates provisioning.SecureBootCertificates, database string) []string {
	switch database {
	case secureBootDatabasePK:
		if certificates.PK == "" {
			return nil
		}

		return []string{certificates.PK}

	case api.SecureBootDatabaseKEK:
		return certificates.KEK

	case api.SecureBootDatabaseDB:
		return certificates.DB

	case api.SecureBootDatabaseDBX:
		return certificates.DBX
	}

	return nil
}

// buildESP assembles the EFI system partition of the enrollment media in dir
// and returns the path of the image file holding it.
func (m *Media) buildESP(ctx context.Context, dir string, loader bootLoader, certificates provisioning.SecureBootCertificates) (string, error) {
	root := filepath.Join(dir, "esp")

	err := os.MkdirAll(filepath.Join(root, "EFI", "BOOT"), 0o700)
	if err != nil {
		return "", fmt.Errorf("Failed to prepare the secure boot enrollment media: %w", err)
	}

	err = os.MkdirAll(filepath.Join(root, "loader", "keys", "auto"), 0o700)
	if err != nil {
		return "", fmt.Errorf("Failed to prepare the secure boot enrollment media: %w", err)
	}

	err = m.copyBootLoader(root, loader)
	if err != nil {
		return "", err
	}

	err = os.WriteFile(filepath.Join(root, "loader", "loader.conf"), []byte(loaderConf), 0o600)
	if err != nil {
		return "", fmt.Errorf("Failed to write the boot loader configuration: %w", err)
	}

	signingCert, signingKey, err := m.signingKeyPair(dir)
	if err != nil {
		return "", err
	}

	for _, database := range secureBootDatabases {
		err = m.writeKeyDatabase(ctx, dir, root, database, certificatesOf(certificates, database), signingCert, signingKey)
		if err != nil {
			return "", err
		}
	}

	err = fixTimes(root)
	if err != nil {
		return "", err
	}

	return m.formatESP(ctx, dir, root)
}

// copyBootLoader installs the unsigned boot loader as the removable media boot
// path, which is the only binary the enrollment media boots.
func (m *Media) copyBootLoader(root string, loader bootLoader) error {
	source := filepath.Join(m.bootLoaderDir, loader.binary)

	body, err := os.ReadFile(source)
	if err != nil {
		if os.IsNotExist(err) {
			return errBootLoaderNotInstalled(source)
		}

		return fmt.Errorf("Failed to read the boot loader %q: %w", source, err)
	}

	target := filepath.Join(root, "EFI", "BOOT", loader.bootPath)

	err = os.WriteFile(target, body, 0o600)
	if err != nil {
		return fmt.Errorf("Failed to install the boot loader into the secure boot enrollment media: %w", err)
	}

	return nil
}

// writeKeyDatabase turns the certificates of a single UEFI key database into the
// signed update systemd-boot enrolls.
func (m *Media) writeKeyDatabase(ctx context.Context, dir string, root string, database string, certificates []string, signingCert string, signingKey string) error {
	if len(certificates) == 0 {
		return nil
	}

	listFile := filepath.Join(dir, database+".esl")

	list, err := os.OpenFile(listFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("Failed to create the signature list of the secure boot key database %q: %w", database, err)
	}

	defer func() { _ = list.Close() }()

	// Multiple signature lists can be concatenated to hold more than one
	// certificate per key database.
	for i, certificate := range certificates {
		certFile := filepath.Join(dir, fmt.Sprintf("%s-%d.pem", database, i))

		err = os.WriteFile(certFile, []byte(certificate), 0o600)
		if err != nil {
			return fmt.Errorf("Failed to write the certificate of the secure boot key database %q: %w", database, err)
		}

		entryFile := filepath.Join(dir, fmt.Sprintf("%s-%d.esl", database, i))

		err = m.runTool(ctx, "cert-to-efi-sig-list", "-g", ownerGUID, certFile, entryFile)
		if err != nil {
			return fmt.Errorf("Failed to build the signature list of the secure boot key database %q: %w", database, err)
		}

		entry, err := os.ReadFile(entryFile)
		if err != nil {
			return fmt.Errorf("Failed to read the signature list of the secure boot key database %q: %w", database, err)
		}

		_, err = list.Write(entry)
		if err != nil {
			return fmt.Errorf("Failed to write the signature list of the secure boot key database %q: %w", database, err)
		}
	}

	err = list.Close()
	if err != nil {
		return fmt.Errorf("Failed to write the signature list of the secure boot key database %q: %w", database, err)
	}

	authFile := filepath.Join(root, "loader", "keys", "auto", database+".auth")

	err = m.runTool(ctx, "sign-efi-sig-list",
		"-g", ownerGUID,
		"-t", authTimestamp,
		"-c", signingCert,
		"-k", signingKey,
		database, listFile, authFile,
	)
	if err != nil {
		return fmt.Errorf("Failed to sign the update of the secure boot key database %q: %w", database, err)
	}

	return nil
}

// signingKeyPair writes the throw away key the key database updates are signed
// with and returns the paths of the certificate and the key.
//
// The signature is never verified: the enrollment only happens while the server
// is in secure boot setup mode, where the firmware accepts an update without
// checking it, and the certificate ending up enrolled is the one inside the
// signature list, never the one signing it.
func (m *Media) signingKeyPair(dir string) (string, string, error) {
	key, err := m.newSigningKey()
	if err != nil {
		return "", "", fmt.Errorf("Failed to generate the signing key of the secure boot enrollment media: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: signingKeyCommonName},
		NotBefore:             time.Unix(0, 0).UTC(),
		NotAfter:              time.Unix(0, 0).UTC().AddDate(100, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return "", "", fmt.Errorf("Failed to create the signing certificate of the secure boot enrollment media: %w", err)
	}

	certFile := filepath.Join(dir, "signing.crt")

	err = os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)
	if err != nil {
		return "", "", fmt.Errorf("Failed to write the signing certificate of the secure boot enrollment media: %w", err)
	}

	keyFile := filepath.Join(dir, "signing.key")

	err = os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: x509MarshalKey(key)}), 0o600)
	if err != nil {
		return "", "", fmt.Errorf("Failed to write the signing key of the secure boot enrollment media: %w", err)
	}

	return certFile, keyFile, nil
}

func x509MarshalKey(key *rsa.PrivateKey) []byte {
	// A generated RSA key always marshals, so the error can not happen.
	der, _ := x509.MarshalPKCS8PrivateKey(key)

	return der
}

// formatESP puts the assembled directory tree into a FAT formatted image and
// returns its path.
func (m *Media) formatESP(ctx context.Context, dir string, root string) (string, error) {
	image := filepath.Join(dir, "esp.img")

	err := os.WriteFile(image, nil, 0o600)
	if err != nil {
		return "", fmt.Errorf("Failed to create the EFI system partition: %w", err)
	}

	err = os.Truncate(image, espSize)
	if err != nil {
		return "", fmt.Errorf("Failed to size the EFI system partition: %w", err)
	}

	err = m.runTool(ctx, "mkfs.vfat", "-S", "512", "-i", espVolumeID, image)
	if err != nil {
		return "", fmt.Errorf("Failed to format the EFI system partition: %w", err)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("Failed to read the contents of the EFI system partition: %w", err)
	}

	// -m preserves the modification times, which fixTimes has normalized.
	args := []string{"-s", "-m", "-i", image}
	for _, entry := range entries {
		args = append(args, filepath.Join(root, entry.Name()))
	}

	err = m.runTool(ctx, "mcopy", append(args, "::/")...)
	if err != nil {
		return "", fmt.Errorf("Failed to fill the EFI system partition: %w", err)
	}

	return image, nil
}

// fixTimes gives every file of the EFI system partition the same modification
// time, so that the generated image only depends on the certificates.
func fixTimes(root string) error {
	stamp, err := time.Parse(time.RFC3339, espFileTime)
	if err != nil {
		return fmt.Errorf("Invalid modification time for the secure boot enrollment media: %w", err)
	}

	err = filepath.Walk(root, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		return os.Chtimes(path, stamp, stamp)
	})
	if err != nil {
		return fmt.Errorf("Failed to normalize the contents of the secure boot enrollment media: %w", err)
	}

	return nil
}

// buildISO wraps the EFI system partition in a partition table and returns the
// path of the enrollment media.
//
// The table is laid out for 2048 byte logical blocks, since a BMC presents the
// media as a CD, which is read in blocks of that size: a table laid out for the
// 512 byte blocks of a disk is not found by the firmware at all.
func (m *Media) buildISO(ctx context.Context, dir string, id string, espFile string) (string, error) {
	definitionsDir := filepath.Join(dir, "repart.d")

	err := os.MkdirAll(definitionsDir, 0o700)
	if err != nil {
		return "", fmt.Errorf("Failed to prepare the partition definitions of the secure boot enrollment media: %w", err)
	}

	definition := fmt.Sprintf(espDefinition, espFile, espSize, espSize)

	err = os.WriteFile(filepath.Join(definitionsDir, "10-esp.conf"), []byte(definition), 0o600)
	if err != nil {
		return "", fmt.Errorf("Failed to write the partition definitions of the secure boot enrollment media: %w", err)
	}

	isoFile := filepath.Join(dir, "media"+mediaExt)

	err = m.runTool(ctx, "systemd-repart",
		// Without it the definitions of the host are used, which on IncusOS are
		// the ones of the operating system image itself.
		"--definitions="+definitionsDir,
		"--empty=create",
		"--size=auto",
		"--sector-size="+strconv.Itoa(isoBlockSize),
		// The media holds neither an encrypted nor a verity partition, so it is
		// built without ever attaching a loop device.
		"--offline=true",
		// Every identifier of the table is derived from the seed, which is what
		// keeps the very same certificates producing the very same image.
		"--seed="+mediaSeed(id).String(),
		// systemd-repart performs a dry run unless it is told not to.
		"--dry-run=no",
		"--json=off",
		"--no-pager",
		isoFile,
	)
	if err != nil {
		return "", fmt.Errorf("Failed to build the secure boot enrollment media: %w", err)
	}

	return isoFile, nil
}

// isoBlockSize is the logical block size of the generated media, which is the
// block size a CD is read in.
const isoBlockSize = 2048

// espDefinition describes the single EFI system partition of the enrollment
// media. CopyBlocks puts the formatted partition into the image verbatim.
const espDefinition = `[Partition]
Type=esp
Label=ESP
CopyBlocks=%s
SizeMinBytes=%d
SizeMaxBytes=%d
`

// mediaSeed derives the seed, that systemd-repart derives every identifier of
// the partition table from, from the ID of the image.
func mediaSeed(id string) uuid.UUID {
	sum := sha256.Sum256([]byte(id))

	var seed uuid.UUID

	copy(seed[:], sum[:])

	// Mark it as a version 4 variant 1 UUID, so it is well formed.
	seed[6] = seed[6]&0x0F | 0x40
	seed[8] = seed[8]&0x3F | 0x80

	return seed
}
