// Package securebootmedia generates the secure boot enrollment media, a minimal
// bootable image, that enrolls the UEFI key databases of a server, whose BMC can
// not modify them itself via Redfish API.
//
// The image holds nothing but an unsigned boot loader and the key database
// updates it enrolls. It therefore only ever boots on a server, whose secure
// boot is in setup mode, where the enrollment happens, or disabled, where it
// does nothing at all.
package securebootmedia

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lxc/incus-os/incus-osd/api/images"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/shared/api"
)

const partialExt = ".partial"

var mediaIDRegexp = regexp.MustCompile(`^[A-Za-z0-9_-]{12}$`)

var errCacheMiss = errors.New("Secure boot enrollment media is not cached")

// requiredTools are the external tools the generation shells out to.
var requiredTools = []string{"cert-to-efi-sig-list", "sign-efi-sig-list", "mkfs.vfat", "mcopy", "systemd-repart"}

// Media generates the secure boot enrollment media and stores it in a directory.
type Media struct {
	dir           string
	bootLoaderDir string

	runTool       func(ctx context.Context, name string, args ...string) error
	lookPath      func(name string) (string, error)
	newSigningKey func() (*rsa.PrivateKey, error)

	mu      sync.Mutex
	entries map[string]*cacheEntry
}

// cacheEntry tracks one image of the cache.
type cacheEntry struct {
	// generating is non nil while a caller is generating the image and is closed
	// as soon as it is done, successfully or not.
	generating chan struct{}

	// lastAccess is the time the image has been handed out for the last time. It
	// is zero for an entry, which has not been accessed since the last restart,
	// in which case the modification time of the file is used instead.
	lastAccess time.Time
}

var _ provisioning.SecureBootMediaPort = &Media{}

type Option func(m *Media)

// New returns the generator, storing the generated media in dir.
func New(dir string, opts ...Option) *Media {
	media := &Media{
		dir:           dir,
		bootLoaderDir: bootLoaderDir,
		entries:       map[string]*cacheEntry{},
		runTool:       runTool,
		lookPath:      exec.LookPath,
		newSigningKey: func() (*rsa.PrivateKey, error) {
			return rsa.GenerateKey(rand.Reader, 2048)
		},
	}

	for _, opt := range opts {
		opt(media)
	}

	return media
}

// bootLoader names the boot loader an architecture boots: the binary shipped by
// the systemd-boot-efi package and the removable media boot path it has to be
// installed as.
type bootLoader struct {
	binary   string
	bootPath string
}

var bootLoaders = map[images.UpdateFileArchitecture]bootLoader{
	images.UpdateFileArchitecture64BitX86: {binary: "systemd-bootx64.efi", bootPath: "BOOTX64.EFI"},
	// images.UpdateFileArchitecture64BitARM: {binary: "systemd-bootaa64.efi", bootPath: "BOOTAA64.EFI"},
}

const bootLoaderDir = "/usr/lib/systemd/boot/efi"

// bootLoaderForArchitecture returns the boot loader of an architecture.
func bootLoaderForArchitecture(architecture images.UpdateFileArchitecture) (bootLoader, error) {
	loader, ok := bootLoaders[architecture]
	if !ok {
		return bootLoader{}, domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Generating the secure boot enrollment media is not supported for the architecture %q", architecture).
			WithDetail("architecture", string(architecture))
	}

	return loader, nil
}

// CheckSupported reports, whether the enrollment media can be generated at all:
// the image type and the architecture are known, the boot loader is installed
// and the external tools the generation shells out to are available.
func (m *Media) CheckSupported(_ context.Context, imageType api.ImageType, architecture images.UpdateFileArchitecture) error {
	_, err := blockSizeForImageType(imageType)
	if err != nil {
		return err
	}

	loader, err := bootLoaderForArchitecture(architecture)
	if err != nil {
		return err
	}

	source := filepath.Join(m.bootLoaderDir, loader.binary)

	_, err = os.Stat(source)
	if err != nil {
		if os.IsNotExist(err) {
			return errBootLoaderNotInstalled(source)
		}

		return fmt.Errorf("Failed to read the boot loader %q: %w", source, err)
	}

	for _, tool := range requiredTools {
		_, err := m.lookPath(tool)
		if err == nil {
			continue
		}

		if errors.Is(err, exec.ErrNotFound) {
			return errToolNotInstalled(tool)
		}

		return fmt.Errorf("Failed to look up %q: %w", tool, err)
	}

	return nil
}

// Generate stores the enrollment media for the certificates, unless it is stored
// already, and returns the ID addressing it.
//
// The call blocks until the image is stored. Concurrent calls for the same
// certificates generate it only once.
func (m *Media) Generate(ctx context.Context, imageType api.ImageType, architecture images.UpdateFileArchitecture, certificates provisioning.SecureBootCertificates) (string, error) {
	_, err := blockSizeForImageType(imageType)
	if err != nil {
		return "", err
	}

	loader, err := bootLoaderForArchitecture(architecture)
	if err != nil {
		return "", err
	}

	if certificates.IsEmpty() {
		return "", domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Generating the secure boot enrollment media is not possible, there are no certificates to enroll")
	}

	if certificates.PK == "" {
		return "", domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Generating the secure boot enrollment media is not possible, the platform key is missing")
	}

	id := MediaID(imageType, architecture, certificates)

	for {
		_, err := os.Stat(m.filename(imageType, id))
		if err == nil {
			m.touch(ctx, imageType, id)

			return id, nil
		}

		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("Failed to look for the cached secure boot enrollment media %q: %w", id, err)
		}

		wait, release := m.acquire(id)
		if release == nil {
			// The very same image is already being generated, wait for it
			// instead of generating it a second time.
			select {
			case <-ctx.Done():
				return "", ctx.Err()

			case <-wait:
			}

			continue
		}

		err = func() error {
			defer release()

			return m.generate(ctx, imageType, id, loader, certificates)
		}()
		if err != nil {
			return "", err
		}

		m.touch(ctx, imageType, id)

		return id, nil
	}
}

// Open returns the already generated enrollment media of an image type
// addressed by id. An image being generated right now is waited for.
func (m *Media) Open(ctx context.Context, imageType api.ImageType, id string) (*provisioning.SecureBootMediaImage, error) {
	if !mediaIDRegexp.MatchString(id) || !imageType.IsValid() {
		return nil, domain.NewErrorf(domain.ErrNotFound, "", "No secure boot enrollment media %q is available", id).
			WithDetail("id", id)
	}

	for {
		image, err := m.open(ctx, imageType, id)
		if err == nil {
			return image, nil
		}

		if !errors.Is(err, errCacheMiss) {
			return nil, err
		}

		wait := m.generating(id)
		if wait == nil {
			return nil, domain.NewErrorf(domain.ErrNotFound, "", "No secure boot enrollment media %q is available", id).
				WithDetail("id", id)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-wait:
		}
	}
}

// Prune removes the generated media, which has not been accessed within ttl,
// together with leftovers of interrupted generations. A media named by inUse is
// kept however old it is.
func (m *Media) Prune(ctx context.Context, ttl time.Duration, inUse []string) error {
	dirEntries, err := os.ReadDir(m.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("Failed to read the secure boot enrollment media directory %q: %w", m.dir, err)
	}

	var errs []error

	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			continue
		}

		filename := dirEntry.Name()

		fileInfo, err := dirEntry.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}

			errs = append(errs, err)
			continue
		}

		id, isMedia := mediaIDFromFilename(filename)

		switch {
		case strings.HasSuffix(filename, partialExt):
			if time.Since(fileInfo.ModTime()) < ttl {
				continue
			}

		case isMedia:
			if slices.Contains(inUse, id) {
				continue
			}

			keep, err := m.keep(id, fileInfo.ModTime(), ttl)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if keep {
				continue
			}

		default:
			// Not something the generator has written, leave it alone.
			continue
		}

		err = os.Remove(filepath.Join(m.dir, filename))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}

		if ctx.Err() != nil {
			errs = append(errs, ctx.Err())
			break
		}
	}

	return errors.Join(errs...)
}

// generate builds the image and stores it under the name it is addressed by.
func (m *Media) generate(ctx context.Context, imageType api.ImageType, id string, loader bootLoader, certificates provisioning.SecureBootCertificates) (err error) {
	workDir, err := os.MkdirTemp("", "operations-center-secure-boot-media-*")
	if err != nil {
		return fmt.Errorf("Failed to create the working directory for the secure boot enrollment media: %w", err)
	}

	defer func() {
		err = errors.Join(err, os.RemoveAll(workDir))
	}()

	espFile, err := m.buildESP(ctx, workDir, loader, certificates)
	if err != nil {
		return err
	}

	imageFile, err := m.buildImage(ctx, workDir, imageType, id, espFile)
	if err != nil {
		return err
	}

	image, err := os.Open(imageFile)
	if err != nil {
		return fmt.Errorf("Failed to open the secure boot enrollment media: %w", err)
	}

	defer func() { _ = image.Close() }()

	return m.write(imageType, id, func(w io.Writer) error {
		_, err := io.Copy(w, image)
		if err != nil {
			return fmt.Errorf("Failed to store the secure boot enrollment media: %w", err)
		}

		return nil
	})
}

// write stores the image, going through a partial file, so an interrupted
// generation never leaves a truncated image behind.
func (m *Media) write(imageType api.ImageType, id string, body func(io.Writer) error) (err error) {
	err = os.MkdirAll(m.dir, 0o700)
	if err != nil {
		return fmt.Errorf("Failed to create the secure boot enrollment media directory %q: %w", m.dir, err)
	}

	partial, err := os.CreateTemp(m.dir, id+".*"+partialExt)
	if err != nil {
		return fmt.Errorf("Failed to create the partial file for the secure boot enrollment media %q: %w", id, err)
	}

	partialName := partial.Name()
	closePartial := sync.OnceValue(partial.Close)

	defer func() {
		if err == nil {
			return
		}

		err = errors.Join(err, closePartial())

		if partialName != "" {
			err = errors.Join(err, os.Remove(partialName))
		}
	}()

	err = body(partial)
	if err != nil {
		return err
	}

	err = closePartial()
	if err != nil {
		return fmt.Errorf("Failed to close the partial file for the secure boot enrollment media %q: %w", id, err)
	}

	err = os.Rename(partialName, m.filename(imageType, id))
	if err != nil {
		return fmt.Errorf("Failed to move the secure boot enrollment media %q into place: %w", id, err)
	}

	partialName = ""

	return nil
}

// open returns the stored image, or reports errCacheMiss, if it is not stored.
func (m *Media) open(ctx context.Context, imageType api.ImageType, id string) (*provisioning.SecureBootMediaImage, error) {
	file, err := os.Open(m.filename(imageType, id))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errCacheMiss
		}

		return nil, fmt.Errorf("Failed to open the secure boot enrollment media %q: %w", id, err)
	}

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("Failed to stat the secure boot enrollment media %q: %w", id, err), file.Close())
	}

	m.touch(ctx, imageType, id)

	return &provisioning.SecureBootMediaImage{
		Content:  file,
		Filename: id + imageType.FileExt(),
		Size:     fileInfo.Size(),
		ModTime:  fileInfo.ModTime(),
	}, nil
}

// acquire either returns a channel to wait on, because another caller is
// generating the image already, or a function to release the generation with,
// because the caller has taken it over.
func (m *Media) acquire(id string) (wait <-chan struct{}, release func()) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.entries[id]
	if !ok {
		entry = &cacheEntry{}
		m.entries[id] = entry
	}

	if entry.generating != nil {
		return entry.generating, nil
	}

	generating := make(chan struct{})
	entry.generating = generating

	return nil, func() {
		m.mu.Lock()
		defer m.mu.Unlock()

		entry.generating = nil
		close(generating)
	}
}

// generating returns a channel closed as soon as the image is done being
// generated, or nil, if nobody is generating it.
func (m *Media) generating(id string) <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.entries[id]
	if !ok {
		return nil
	}

	return entry.generating
}

// keep reports, whether a stored image is still in use. modTime is used as the
// last access for an entry, which has not been accessed since the last restart.
func (m *Media) keep(id string, modTime time.Time, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.entries[id]
	if ok && entry.generating != nil {
		return true, nil
	}

	lastAccess := modTime
	if ok && entry.lastAccess.After(lastAccess) {
		lastAccess = entry.lastAccess
	}

	if time.Since(lastAccess) < ttl {
		return true, nil
	}

	delete(m.entries, id)

	return false, nil
}

// touch records, that an image has been used.
//
// The modification time of the file is moved along with the in-memory entry, so
// the last access survives a restart: an entry, that does not, has the image
// fall back to the time it has been generated at and be pruned right away,
// however recently a BMC has read it.
func (m *Media) touch(ctx context.Context, imageType api.ImageType, id string) {
	now := time.Now()

	m.mu.Lock()

	entry, ok := m.entries[id]
	if !ok {
		entry = &cacheEntry{}
		m.entries[id] = entry
	}

	entry.lastAccess = now

	m.mu.Unlock()

	// The image is only missing, where it has just been pruned, in which case
	// there is nothing left to record the access on.
	err := os.Chtimes(m.filename(imageType, id), now, now)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.WarnContext(ctx, "Failed to record the access to the secure boot enrollment media", slog.String("id", id), logger.Err(err))
	}
}

func (m *Media) filename(imageType api.ImageType, id string) string {
	return filepath.Join(m.dir, id+imageType.FileExt())
}

// mediaIDFromFilename returns the ID of the image a stored file holds and
// reports, whether the file is one the generator has written at all.
func mediaIDFromFilename(filename string) (string, bool) {
	extension := filepath.Ext(filename)

	_, ok := api.ImageTypeForFileExt(extension)
	if !ok {
		return "", false
	}

	id := strings.TrimSuffix(filename, extension)
	if !mediaIDRegexp.MatchString(id) {
		return "", false
	}

	return id, true
}

// runTool executes one of the external tools the generation needs, reporting
// what it printed, so a failure names what went wrong.
func runTool(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)

	// The tools write timestamps into what they produce, so the timezone has to
	// be pinned to keep the generated image reproducible.
	cmd.Env = append(os.Environ(), "TZ=UTC")

	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	if errors.Is(err, exec.ErrNotFound) {
		return errToolNotInstalled(name)
	}

	message := strings.TrimSpace(string(output))
	if message == "" {
		return fmt.Errorf("Failed to run %q: %w", name, err)
	}

	return fmt.Errorf("Failed to run %q: %w: %s", name, err, message)
}

// errToolNotInstalled reports an external tool the generation needs as missing.
func errToolNotInstalled(name string) error {
	return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Generating the secure boot enrollment media is not supported, %q is not installed", name).
		WithDetail("tool", name)
}

// errBootLoaderNotInstalled reports the boot loader the media boots as missing.
func errBootLoaderNotInstalled(source string) error {
	return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Generating the secure boot enrollment media is not supported, the boot loader %q is not installed, it is provided by the systemd-boot-efi package", source).
		WithDetail("boot_loader", source)
}
