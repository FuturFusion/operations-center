package securebootmedia_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/securebootcerts"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/securebootmedia"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/shared/api"
)

// testCertificates returns a certificate set covering every key database.
func testCertificates(t *testing.T) provisioning.SecureBootCertificates {
	t.Helper()

	catalog, err := securebootcerts.New()
	require.NoError(t, err, "The certificate catalog has to be available to build a certificate set from it")

	certificates, unknown := catalog.CertificatesByFingerprint(catalog.Fingerprints())
	require.Empty(t, unknown, "The catalog has to know its own fingerprints")
	require.GreaterOrEqual(t, len(certificates), 3, "A certificate set needs three distinct certificates to cover every key database")

	return provisioning.SecureBootCertificates{
		PK:  certificates[0],
		KEK: []string{certificates[1]},
		DB:  []string{certificates[2], certificates[0]},
		DBX: []string{certificates[1]},
	}
}

// skipWithoutTools skips a test, that needs the external tools, where they are
// not installed, so the rest of the suite still runs.
func skipWithoutTools(t *testing.T) {
	t.Helper()

	err := securebootmedia.New(t.TempDir()).CheckSupported(t.Context(), api.ImageTypeISO, images.UpdateFileArchitecture64BitX86)
	if err != nil {
		t.Skipf("The secure boot enrollment media can not be generated here: %v", err)
	}
}

func TestMediaCheckSupported(t *testing.T) {
	bootLoaderDir := t.TempDir()

	err := os.WriteFile(filepath.Join(bootLoaderDir, "systemd-bootx64.efi"), []byte("boot loader"), 0o600)
	require.NoError(t, err)

	allToolsInstalled := func(name string) (string, error) { return "/usr/bin/" + name, nil }

	tests := []struct {
		name          string
		imageType     api.ImageType
		architecture  images.UpdateFileArchitecture
		bootLoaderDir string
		lookPath      func(name string) (string, error)

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:          "everything is in place for an ISO",
			imageType:     api.ImageTypeISO,
			architecture:  images.UpdateFileArchitecture64BitX86,
			bootLoaderDir: bootLoaderDir,
			lookPath:      allToolsInstalled,

			assertErr: require.NoError,
		},
		{
			name:          "everything is in place for a raw image",
			imageType:     api.ImageTypeRaw,
			architecture:  images.UpdateFileArchitecture64BitX86,
			bootLoaderDir: bootLoaderDir,
			lookPath:      allToolsInstalled,

			assertErr: require.NoError,
		},
		{
			name:          "the image type is not defined",
			architecture:  images.UpdateFileArchitecture64BitX86,
			bootLoaderDir: bootLoaderDir,
			lookPath:      allToolsInstalled,

			assertErr: errassert.OperationNotPermittedErrorContains("is not supported for the image type"),
		},
		{
			name:          "the image type is not supported",
			imageType:     api.ImageType("qcow2"),
			architecture:  images.UpdateFileArchitecture64BitX86,
			bootLoaderDir: bootLoaderDir,
			lookPath:      allToolsInstalled,

			assertErr: errassert.OperationNotPermittedErrorContains("is not supported for the image type"),
		},
		{
			name:          "the architecture is not defined",
			imageType:     api.ImageTypeISO,
			architecture:  images.UpdateFileArchitectureUndefined,
			bootLoaderDir: bootLoaderDir,
			lookPath:      allToolsInstalled,

			assertErr: errassert.OperationNotPermittedErrorContains("is not supported for the architecture"),
		},
		{
			name:          "the architecture is not supported",
			imageType:     api.ImageTypeISO,
			architecture:  images.UpdateFileArchitecture64BitARM,
			bootLoaderDir: bootLoaderDir,
			lookPath:      allToolsInstalled,

			assertErr: errassert.OperationNotPermittedErrorContains(`Generating the secure boot enrollment media is not supported for the architecture`),
		},
		{
			name:          "the boot loader is not installed",
			imageType:     api.ImageTypeISO,
			architecture:  images.UpdateFileArchitecture64BitX86,
			bootLoaderDir: t.TempDir(),
			lookPath:      allToolsInstalled,

			assertErr: errassert.OperationNotPermittedErrorContains("is provided by the systemd-boot-efi package"),
		},
		{
			name:          "one of the tools is not installed",
			imageType:     api.ImageTypeISO,
			architecture:  images.UpdateFileArchitecture64BitX86,
			bootLoaderDir: bootLoaderDir,
			lookPath: func(name string) (string, error) {
				if name == "systemd-repart" {
					return "", exec.ErrNotFound
				}

				return allToolsInstalled(name)
			},

			assertErr: errassert.OperationNotPermittedErrorContains(`"systemd-repart" is not installed`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			media := securebootmedia.New(t.TempDir(),
				securebootmedia.WithBootLoaderDir(tc.bootLoaderDir),
				securebootmedia.WithLookPath(tc.lookPath),
			)

			err := media.CheckSupported(t.Context(), tc.imageType, tc.architecture)

			tc.assertErr(t, err)
		})
	}
}

func TestMediaID(t *testing.T) {
	certificates := testCertificates(t)

	id := securebootmedia.MediaID(api.ImageTypeISO, images.UpdateFileArchitecture64BitX86, certificates)
	require.Regexp(t, `^[A-Za-z0-9_-]{12}$`, id, "The media ID has to be safe as a path segment")
	require.Equal(t, id, securebootmedia.MediaID(api.ImageTypeISO, images.UpdateFileArchitecture64BitX86, certificates), "The same media has to be addressed by the same ID")

	require.NotEqual(t, id, securebootmedia.MediaID(api.ImageTypeRaw, images.UpdateFileArchitecture64BitX86, certificates),
		"A media of another image type is laid out for another block size, so it must not share the ID")

	require.NotEqual(t, id, securebootmedia.MediaID(api.ImageTypeISO, images.UpdateFileArchitecture64BitARM, certificates),
		"A media for another architecture holds another boot loader, so it must not share the ID")

	changed := certificates
	changed.DB = []string{certificates.DB[0]}

	require.NotEqual(t, id, securebootmedia.MediaID(api.ImageTypeISO, images.UpdateFileArchitecture64BitX86, changed), "Dropping a certificate has to address a different image")

	reordered := certificates
	reordered.DB = []string{certificates.DB[1], certificates.DB[0]}

	require.NotEqual(t, id, securebootmedia.MediaID(api.ImageTypeISO, images.UpdateFileArchitecture64BitX86, reordered), "The order of the certificates is part of what is enrolled")

	withUpdate := certificates
	withUpdate.PKUpdate = []byte("a signed platform key update")

	require.NotEqual(t, id, securebootmedia.MediaID(api.ImageTypeISO, images.UpdateFileArchitecture64BitX86, withUpdate),
		"The platform key is enrolled from the update, so a change of it has to address another image")
}

func TestMedia_GenerateBootsTheArchitectureOfTheServer(t *testing.T) {
	skipWithoutTools(t)

	certificates := testCertificates(t)

	dir := t.TempDir()
	media := securebootmedia.New(dir)

	id, err := media.Generate(t.Context(), api.ImageTypeISO, images.UpdateFileArchitecture64BitX86, certificates)
	require.NoError(t, err)

	listing := listESP(t, filepath.Join(dir, id+".iso"), isoBlockSize)
	require.Contains(t, listing, "::/EFI/BOOT/BOOTX64.EFI", "the media has to boot the architecture of the server")
	require.NotContains(t, listing, "::/EFI/BOOT/BOOTAA64.EFI", "the media must not hold the boot loader of another architecture")
}

func TestMedia_GenerateRejectsAnUnsupportedArchitecture(t *testing.T) {
	certificates := testCertificates(t)

	for _, architecture := range []images.UpdateFileArchitecture{images.UpdateFileArchitectureUndefined, images.UpdateFileArchitecture64BitARM} {
		media := securebootmedia.New(t.TempDir())

		_, err := media.Generate(t.Context(), api.ImageTypeISO, architecture, certificates)
		require.ErrorIs(t, err, domain.ErrOperationNotPermitted, "An architecture without a boot loader must not be served the boot loader of another one")
	}
}

func TestMedia_Generate(t *testing.T) {
	skipWithoutTools(t)

	tests := []struct {
		name string

		imageType api.ImageType

		wantBlockSize int
	}{
		{
			name: "an ISO is laid out for the blocks of a CD",

			imageType:     api.ImageTypeISO,
			wantBlockSize: isoBlockSize,
		},
		{
			name: "a raw image is laid out for the blocks of a USB stick",

			imageType:     api.ImageTypeRaw,
			wantBlockSize: rawBlockSize,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			media := securebootmedia.New(dir)

			certificates := testCertificates(t)

			id, err := media.Generate(t.Context(), tc.imageType, images.UpdateFileArchitecture64BitX86, certificates)
			require.NoError(t, err)
			require.Equal(t, securebootmedia.MediaID(tc.imageType, images.UpdateFileArchitecture64BitX86, certificates), id, "The generated image has to be addressed by the ID of what it holds")

			image, err := media.Open(t.Context(), tc.imageType, id)
			require.NoError(t, err)

			defer func() { _ = image.Content.Close() }()

			require.Equal(t, id+tc.imageType.FileExt(), image.Filename, "The image has to be served with the file extension a BMC derives the kind of media from")

			body := make([]byte, image.Size)

			_, err = image.Content.Read(body)
			require.NoError(t, err)

			assertValidImage(t, body, tc.wantBlockSize)
			assertHoldsFiles(t, filepath.Join(dir, id+tc.imageType.FileExt()), tc.wantBlockSize)
		})
	}
}

func TestMedia_GenerateIsReproducible(t *testing.T) {
	skipWithoutTools(t)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	certificates := testCertificates(t)

	generate := func() []byte {
		dir := t.TempDir()

		media := securebootmedia.New(dir, securebootmedia.WithSigningKey(key))

		id, err := media.Generate(t.Context(), api.ImageTypeISO, images.UpdateFileArchitecture64BitX86, certificates)
		require.NoError(t, err)

		body, err := os.ReadFile(filepath.Join(dir, id+".iso"))
		require.NoError(t, err)

		return body
	}

	first := generate()
	second := generate()

	require.Equal(t, first, second, "The same certificates and the same signing key have to produce the very same image")
}

func TestMedia_GenerateCaches(t *testing.T) {
	skipWithoutTools(t)

	dir := t.TempDir()
	media := securebootmedia.New(dir)

	certificates := testCertificates(t)

	id, err := media.Generate(t.Context(), api.ImageTypeISO, images.UpdateFileArchitecture64BitX86, certificates)
	require.NoError(t, err)

	filename := filepath.Join(dir, id+".iso")

	err = os.WriteFile(filename, []byte("cached"), 0o600)
	require.NoError(t, err)

	_, err = media.Generate(t.Context(), api.ImageTypeISO, images.UpdateFileArchitecture64BitX86, certificates)
	require.NoError(t, err)

	body, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, []byte("cached"), body, "An image, that is stored already, must not be generated a second time")
}

func TestMedia_GenerateErrors(t *testing.T) {
	certificates := testCertificates(t)

	tests := []struct {
		name string

		imageType    api.ImageType
		certificates provisioning.SecureBootCertificates
		opts         []securebootmedia.Option

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:         "error - image type not supported",
			imageType:    api.ImageType("qcow2"),
			certificates: certificates,
			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:         "error - no certificates at all",
			imageType:    api.ImageTypeISO,
			certificates: provisioning.SecureBootCertificates{},
			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:         "error - platform key missing",
			imageType:    api.ImageTypeISO,
			certificates: provisioning.SecureBootCertificates{KEK: certificates.KEK},
			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:         "error - boot loader not installed",
			imageType:    api.ImageTypeISO,
			certificates: certificates,
			opts:         []securebootmedia.Option{securebootmedia.WithBootLoaderDir(t.TempDir())},
			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:         "error - tool fails",
			imageType:    api.ImageTypeISO,
			certificates: certificates,
			opts: []securebootmedia.Option{
				securebootmedia.WithBootLoaderDir(bootLoaderDirWith(t, "not a real boot loader")),
				securebootmedia.WithRunTool(func(_ context.Context, _ string, _ ...string) error {
					return boom.Error
				}),
			},
			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			media := securebootmedia.New(dir, tc.opts...)

			_, err := media.Generate(t.Context(), tc.imageType, images.UpdateFileArchitecture64BitX86, tc.certificates)
			tc.assertErr(t, err)

			leftovers, err := filepath.Glob(filepath.Join(dir, "*"))
			require.NoError(t, err)
			require.Empty(t, leftovers, "A failed generation must not leave anything behind")
		})
	}
}

func TestMedia_Open(t *testing.T) {
	media := securebootmedia.New(t.TempDir())

	_, err := media.Open(t.Context(), api.ImageTypeISO, "not an ID")
	require.ErrorIs(t, err, domain.ErrNotFound, "An ID, that can not address an image, has to be rejected")

	_, err = media.Open(t.Context(), api.ImageType("qcow2"), "AAAAAAAAAAAA")
	require.ErrorIs(t, err, domain.ErrNotFound, "An image type, that can not be generated, addresses no image")

	_, err = media.Open(t.Context(), api.ImageTypeISO, "AAAAAAAAAAAA")
	require.ErrorIs(t, err, domain.ErrNotFound, "An image, that has not been generated, is not available")
}

func TestMedia_Prune(t *testing.T) {
	dir := t.TempDir()
	media := securebootmedia.New(dir)

	stale := writeInto(t, dir, "AAAAAAAAAAAA.iso", time.Now().Add(-2*time.Hour))
	fresh := writeInto(t, dir, "BBBBBBBBBBBB.iso", time.Now())
	staleRaw := writeInto(t, dir, "EEEEEEEEEEEE.raw", time.Now().Add(-2*time.Hour))
	freshRaw := writeInto(t, dir, "FFFFFFFFFFFF.raw", time.Now())
	stalePartial := writeInto(t, dir, "CCCCCCCCCCCC.1.partial", time.Now().Add(-2*time.Hour))
	freshPartial := writeInto(t, dir, "DDDDDDDDDDDD.1.partial", time.Now())
	foreign := writeInto(t, dir, "unrelated.txt", time.Now().Add(-2*time.Hour))
	foreignID := writeInto(t, dir, "not an ID.iso", time.Now().Add(-2*time.Hour))

	err := media.Prune(t.Context(), time.Hour)
	require.NoError(t, err)

	require.NoFileExists(t, stale, "An image, that has not been accessed within the TTL, has to be removed")
	require.NoFileExists(t, staleRaw, "An image of every type the generator writes has to be pruned")
	require.NoFileExists(t, stalePartial, "The leftover of an interrupted generation has to be removed")
	require.FileExists(t, fresh, "An image within the TTL has to be kept")
	require.FileExists(t, freshRaw, "An image of every type the generator writes has to be kept within the TTL")
	require.FileExists(t, freshPartial, "A generation in flight must not be removed")
	require.FileExists(t, foreign, "A file, that the generator has not written, has to be left alone")
	require.FileExists(t, foreignID, "A file, whose name is no media ID, has to be left alone")
}


	const blockSize = 2048

	require.Zero(t, len(body)%blockSize, "The image has to consist of whole 2048 byte blocks")
// isoBlockSize and rawBlockSize are the logical block sizes the media of an
// image type has to be laid out for: the blocks a CD is read in and the ones a
// disk is read in.
const (
	isoBlockSize = 2048
	rawBlockSize = 512
)

// assertValidImage checks, that the image is a GPT holding a single EFI system
// partition, laid out for blockSize, the way the firmware of a server finds it.
func assertValidImage(t *testing.T, body []byte, blockSize int) {
	t.Helper()

	require.Zero(t, len(body)%blockSize, "The image has to consist of whole %d byte blocks", blockSize)
	require.Equal(t, []byte{0x55, 0xAA}, body[510:512], "The protective master boot record has to carry its signature")
	require.EqualValues(t, 0xEE, body[446+4], "The protective master boot record has to announce a GPT")

	lastLBA := int64(len(body)/blockSize) - 1

	primary := assertValidGPTHeader(t, body, blockSize, 1, "primary")
	backup := assertValidGPTHeader(t, body, blockSize, lastLBA, "backup")

	require.EqualValues(t, lastLBA, binary.LittleEndian.Uint64(primary[32:40]), "The primary header has to point at the backup header")
	require.EqualValues(t, 1, binary.LittleEndian.Uint64(backup[32:40]), "The backup header has to point at the primary header")

	entryLBA := int64(binary.LittleEndian.Uint64(primary[72:80]))
	entries := body[entryLBA*int64(blockSize) : entryLBA*int64(blockSize)+128*128]

	require.Equal(t,
		[]byte{0x28, 0x73, 0x2A, 0xC1, 0x1F, 0xF8, 0xD2, 0x11, 0xBA, 0x4B, 0x00, 0xA0, 0xC9, 0x3E, 0xC9, 0x3B},
		entries[0:16],
		"The image has to hold an EFI system partition",
	)

	start := int64(binary.LittleEndian.Uint64(entries[32:40]))
	end := int64(binary.LittleEndian.Uint64(entries[40:48]))

	require.Less(t, start, end, "The EFI system partition has to hold something")
	require.LessOrEqual(t, end, int64(binary.LittleEndian.Uint64(primary[48:56])), "The EFI system partition has to fit into the usable range")

	partition := body[start*int64(blockSize) : (end+1)*int64(blockSize)]
	require.Equal(t, []byte("mkfs.fat"), partition[3:11], "The EFI system partition has to hold a FAT filesystem")
}

func assertValidGPTHeader(t *testing.T, body []byte, blockSize int, lba int64, name string) []byte {
	t.Helper()

	header := body[lba*int64(blockSize) : lba*int64(blockSize)+92]

	require.Equal(t, []byte("EFI PART"), header[0:8], "The %s GPT header has to carry its signature", name)
	require.EqualValues(t, lba, binary.LittleEndian.Uint64(header[24:32]), "The %s GPT header has to know where it is", name)

	checksummed := make([]byte, len(header))
	copy(checksummed, header)
	clear(checksummed[16:20])

	require.Equal(t, binary.LittleEndian.Uint32(header[16:20]), crc32.ChecksumIEEE(checksummed), "The %s GPT header checksum has to match", name)

	entryLBA := int64(binary.LittleEndian.Uint64(header[72:80]))
	count := int64(binary.LittleEndian.Uint32(header[80:84]))
	size := int64(binary.LittleEndian.Uint32(header[84:88]))
	entries := body[entryLBA*int64(blockSize) : entryLBA*int64(blockSize)+count*size]

	require.Equal(t, binary.LittleEndian.Uint32(header[88:92]), crc32.ChecksumIEEE(entries), "The %s partition entry checksum has to match", name)

	return header
}

// listESP returns the paths the EFI system partition of an image holds.
func listESP(t *testing.T, filename string, blockSize int) string {
	t.Helper()

	body, err := os.ReadFile(filename)
	require.NoError(t, err)

	offset := espOffset(t, body, blockSize)

	listing, err := exec.Command("mdir", "-/", "-b", "-i", fmt.Sprintf("%s@@%d", filename, offset), "::/").CombinedOutput() //nolint:noctx
	require.NoError(t, err, "The EFI system partition has to be readable: %s", string(listing))

	return string(listing)
}

// espOffset returns the byte offset the EFI system partition starts at, as the
// partition table of the image names it. Where the partition ends up is
// systemd-repart's decision, so it is read back instead of being assumed.
func espOffset(t *testing.T, body []byte, blockSize int) int64 {
	t.Helper()

	primary := body[blockSize : blockSize+92]

	entryLBA := int64(binary.LittleEndian.Uint64(primary[72:80]))
	entrySize := int64(binary.LittleEndian.Uint32(primary[84:88]))
	entry := body[entryLBA*int64(blockSize) : entryLBA*int64(blockSize)+entrySize]

	return int64(binary.LittleEndian.Uint64(entry[32:40])) * int64(blockSize)
}

// bootLoaderDirWith returns a directory holding a stand in for every boot loader.
func bootLoaderDirWith(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()

	for _, loader := range []string{"systemd-bootx64.efi", "systemd-bootaa64.efi"} {
		err := os.WriteFile(filepath.Join(dir, loader), []byte(body), 0o600)
		require.NoError(t, err)
	}

	return dir
}

// assertHoldsFiles checks, that the EFI system partition holds what makes
// systemd-boot enroll the key databases.
func assertHoldsFiles(t *testing.T, filename string, blockSize int) {
	t.Helper()

	listing := listESP(t, filename, blockSize)

	for _, want := range []string{
		"::/EFI/BOOT/BOOTX64.EFI",
		"::/loader/loader.conf",
		"::/loader/keys/auto/PK.auth",
		"::/loader/keys/auto/KEK.auth",
		"::/loader/keys/auto/db.auth",
		"::/loader/keys/auto/dbx.auth",
	} {
		require.Contains(t, listing, want, "The enrollment media has to hold %q", want)
	}
}

func writeInto(t *testing.T, dir string, name string, modTime time.Time) string {
	t.Helper()

	filename := filepath.Join(dir, name)

	err := os.WriteFile(filename, []byte("x"), 0o600)
	require.NoError(t, err)

	err = os.Chtimes(filename, modTime, modTime)
	require.NoError(t, err)

	return filename
}
