package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/flasher"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	provisioningToken "github.com/FuturFusion/operations-center/internal/provisioning/token"
	"github.com/FuturFusion/operations-center/shared/api"
)

// Test_tokenSeedImageGet_servedFromCache wires the real token service and the
// real flasher behind the HTTP route, to cover what mocking either of them
// cannot: that a request for an image ends up being answered from a cache file,
// and that all the byte ranges after the first request are served without
// touching the compressed image again.
func Test_tokenSeedImageGet_servedFromCache(t *testing.T) {
	const isoGzFilename = "IncusOS_1.iso.gz"

	tokenID := uuid.MustParse(tokenUUID)
	updateUUID := uuid.MustParse("6c1ea9a2-8f1c-4a4f-9f2d-3e5b7c9d1a20")

	// An image, which is large enough for ranges to be interesting, but small
	// enough to keep the test cheap.
	imageContent := strings.Repeat("incus-os-0123456789", 2500) + strings.Repeat("\x00", 64*1024) + strings.Repeat("incus-os-0123456789", 2500)

	var compressed bytes.Buffer

	gzipWriter := gzip.NewWriter(&compressed)

	_, err := gzipWriter.Write([]byte(imageContent))
	require.NoError(t, err)
	require.NoError(t, gzipWriter.Close())

	cacheDir := filepath.Join(t.TempDir(), "seed-images")

	requireSpaceForCachedImage(t, filepath.Dir(cacheDir))

	certPEM, keyPEM, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	imageFlasher := flasher.New("https://operations-center.local:7443", cert, flasher.WithCacheDir(cacheDir))

	repo := &repoMock.TokenRepoMock{
		GetByUUIDFunc: func(ctx context.Context, id uuid.UUID) (*provisioning.Token, error) {
			return &provisioning.Token{UUID: id}, nil
		},
		GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
			return &provisioning.TokenSeed{Name: name, Public: true, LastUpdated: time.Now()}, nil
		},
	}

	// decompressions counts how often the compressed image is actually read,
	// which is the work the cache exists to avoid.
	var decompressions atomic.Int64

	updateSvc := &svcMock.UpdateServiceMock{
		GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
			return provisioning.Updates{{UUID: updateUUID}}, nil
		},
		GetUpdateAllFilesFunc: func(ctx context.Context, id uuid.UUID) (provisioning.UpdateFiles, error) {
			return provisioning.UpdateFiles{
				{
					Filename:     isoGzFilename,
					Type:         images.UpdateFileTypeImageISO,
					Architecture: images.UpdateFileArchitecture64BitX86,
				},
			}, nil
		},
		GetUpdateFileByFilenameFunc: func(ctx context.Context, id uuid.UUID, filename string) (io.ReadCloser, int, error) {
			return &readCountingReadCloser{
				Reader: bytes.NewReader(compressed.Bytes()),
				reads:  &decompressions,
			}, compressed.Len(), nil
		},
	}

	channelSvc := &svcMock.ChannelServiceMock{
		GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
			return &provisioning.Channel{Name: name}, nil
		},
	}

	client := &adapterMock.TokenClientPortMock{
		GetSecurityConfigFunc: func(ctx context.Context, server provisioning.Server) (provisioning.ServerSystemSecurity, error) {
			return provisioning.ServerSystemSecurity{}, nil
		},
	}

	tokenSvc := provisioningToken.New(repo, updateSvc, channelSvc, imageFlasher, client)

	// A "file.iso" filename asks for whatever image the token seed resolves to,
	// which is what the CLI and the UI do.
	path := "/" + tokenUUID + "/seeds/test/architecture/x86_64/channel/stable/type/iso/file.iso"

	cacheID := provisioning.SeedImageCacheID(tokenID, "test", api.ImageTypeISO, images.UpdateFileArchitecture64BitX86, "stable")

	t.Run("the first request generates the image and serves it in full", func(t *testing.T) {
		body, resp := doTokenRequestFull(t, tokenSvc, http.MethodGet, path, "", nil)

		require.Equal(t, http.StatusOK, resp.statusCode)
		require.Equal(t, imageContent, body)

		// The token seed is public, so the generated image is stored where a
		// request naming it can reach it.
		images := cachedImages(t, cacheDir, true)
		require.Len(t, images, 1)
		require.True(t, strings.HasPrefix(images[0], cacheID+"-"), images[0])
	})

	t.Run("every range afterwards is served from the cache file", func(t *testing.T) {
		ranges := []struct {
			header string
			from   int
			to     int
		}{
			{header: "bytes=0-9", from: 0, to: 10},
			{header: "bytes=50000-50009", from: 50000, to: 50010},
			// A BMC jumps back and forth, which is what makes generating the
			// image on the fly expensive.
			{header: "bytes=10-19", from: 10, to: 20},
		}

		for _, r := range ranges {
			body, resp := doTokenRequestFull(t, tokenSvc, http.MethodGet, path, "", http.Header{"Range": []string{r.header}})

			require.Equal(t, http.StatusPartialContent, resp.statusCode, r.header)
			require.Equal(t, imageContent[r.from:r.to], body, r.header)
		}

		require.Equal(t, int64(1), decompressions.Load(), "the compressed image must only be read for the first request")
	})
}

func Test_tokenSeedImageGet_preparedImage(t *testing.T) {
	const isoGzFilename = "IncusOS_1.iso.gz"

	tokenID := uuid.MustParse(tokenUUID)
	updateUUID := uuid.MustParse("6c1ea9a2-8f1c-4a4f-9f2d-3e5b7c9d1a20")

	imageContent := strings.Repeat("incus-os-0123456789", 2500) + strings.Repeat("\x00", 64*1024) + strings.Repeat("incus-os-0123456789", 2500)

	var compressed bytes.Buffer

	gzipWriter := gzip.NewWriter(&compressed)

	_, err := gzipWriter.Write([]byte(imageContent))
	require.NoError(t, err)
	require.NoError(t, gzipWriter.Close())

	cacheDir := filepath.Join(t.TempDir(), "seed-images")

	requireSpaceForCachedImage(t, filepath.Dir(cacheDir))

	certPEM, keyPEM, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	imageFlasher := flasher.New("https://operations-center.local:7443", cert, flasher.WithCacheDir(cacheDir))

	var lookups atomic.Int64

	public := true

	repo := &repoMock.TokenRepoMock{
		GetByUUIDFunc: func(ctx context.Context, id uuid.UUID) (*provisioning.Token, error) {
			lookups.Add(1)

			return &provisioning.Token{UUID: id}, nil
		},
		GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
			lookups.Add(1)

			return &provisioning.TokenSeed{Name: name, Public: public, LastUpdated: time.Now()}, nil
		},
	}

	updateSvc := &svcMock.UpdateServiceMock{
		GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
			lookups.Add(1)

			return provisioning.Updates{{UUID: updateUUID}}, nil
		},
		GetUpdateAllFilesFunc: func(ctx context.Context, id uuid.UUID) (provisioning.UpdateFiles, error) {
			lookups.Add(1)

			return provisioning.UpdateFiles{
				{
					Filename:     isoGzFilename,
					Type:         images.UpdateFileTypeImageISO,
					Architecture: images.UpdateFileArchitecture64BitX86,
				},
			}, nil
		},
		GetUpdateFileByFilenameFunc: func(ctx context.Context, id uuid.UUID, filename string) (io.ReadCloser, int, error) {
			lookups.Add(1)

			return io.NopCloser(bytes.NewReader(compressed.Bytes())), compressed.Len(), nil
		},
	}

	channelSvc := &svcMock.ChannelServiceMock{
		GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
			lookups.Add(1)

			return &provisioning.Channel{Name: name}, nil
		},
	}

	client := &adapterMock.TokenClientPortMock{
		GetSecurityConfigFunc: func(ctx context.Context, server provisioning.Server) (provisioning.ServerSystemSecurity, error) {
			lookups.Add(1)

			return provisioning.ServerSystemSecurity{}, nil
		},
	}

	tokenSvc := provisioningToken.New(repo, updateSvc, channelSvc, imageFlasher, client)

	ctx := context.Background()

	fingerprintID, err := tokenSvc.ResolveTokenSeedImageID(ctx, tokenID, "test", api.ImageTypeISO, images.UpdateFileArchitecture64BitX86, "stable")
	require.NoError(t, err)
	require.NoError(t, tokenSvc.PrepareTokenSeedImage(ctx, tokenID, "test", api.ImageTypeISO, images.UpdateFileArchitecture64BitX86, "stable"))

	basePath := "/" + tokenUUID + "/seeds/test/architecture/x86_64/channel/stable/type/iso/"
	path := basePath + fingerprintID + ".iso"

	t.Run("every range of the prepared image is served without a single lookup", func(t *testing.T) {
		lookups.Store(0)

		for i := range 32 {
			from := i * 512
			to := from + 511

			body, resp := doTokenRequestFull(t, tokenSvc, http.MethodGet, path, "",
				http.Header{"Range": []string{fmt.Sprintf("bytes=%d-%d", from, to)}})

			require.Equal(t, http.StatusPartialContent, resp.statusCode)
			require.Equal(t, imageContent[from:to+1], body)
		}

		require.Equal(t, int64(0), lookups.Load(), "a request naming a prepared image must not look anything up")
	})

	t.Run("the whole image is served with a stable validator", func(t *testing.T) {
		lookups.Store(0)

		body, resp := doTokenRequestFull(t, tokenSvc, http.MethodGet, path, "", nil)
		require.Equal(t, http.StatusOK, resp.statusCode)
		require.Equal(t, imageContent, body)
		require.Equal(t, "bytes", resp.header.Get("Accept-Ranges"))

		lastModified := resp.header.Get("Last-Modified")
		require.NotEmpty(t, lastModified)

		body, resp = doTokenRequestFull(t, tokenSvc, http.MethodGet, path, "", http.Header{
			"Range":    []string{"bytes=10-19"},
			"If-Range": []string{lastModified},
		})

		require.Equal(t, http.StatusPartialContent, resp.statusCode)
		require.Equal(t, imageContent[10:20], body)

		require.Equal(t, int64(0), lookups.Load())
	})

	t.Run("an image, which has never been generated, is not found", func(t *testing.T) {
		lookups.Store(0)

		_, resp := doTokenRequestFull(t, tokenSvc, http.MethodGet, basePath+"0123456789ab.iso", "", nil)
		require.Equal(t, http.StatusNotFound, resp.statusCode)

		require.Equal(t, int64(0), lookups.Load())
	})

	t.Run("a filename not naming a generated image asks for the image of the token seed", func(t *testing.T) {
		for _, filename := range []string{"file.iso", "short.iso", "way-too-long-to-be-an-id.iso"} {
			lookups.Store(0)

			body, resp := doTokenRequestFull(t, tokenSvc, http.MethodGet, basePath+filename, "", nil)
			require.Equal(t, http.StatusOK, resp.statusCode, filename)
			require.Equal(t, imageContent, body, filename)

			require.NotZero(t, lookups.Load(), filename)
		}

		_, resp := doTokenRequestFull(t, tokenSvc, http.MethodGet, basePath+"..%2f..%2fetc%2fpasswd", "", nil)
		require.Equal(t, http.StatusBadRequest, resp.statusCode)
	})

	t.Run("an image of a token seed, which is not public, gets no address of its own", func(t *testing.T) {
		public = false

		_, err := tokenSvc.ResolveTokenSeedImageID(ctx, tokenID, "private", api.ImageTypeISO, images.UpdateFileArchitecture64BitX86, "stable")
		require.ErrorIs(t, err, domain.ErrOperationNotPermitted)

		require.ErrorIs(t, tokenSvc.PrepareTokenSeedImage(ctx, tokenID, "private", api.ImageTypeISO, images.UpdateFileArchitecture64BitX86, "stable"), domain.ErrOperationNotPermitted)

		before := cachedImages(t, cacheDir, true)

		body, resp := doTokenRequestFull(t, tokenSvc, http.MethodGet, "/"+tokenUUID+"/seeds/private/architecture/x86_64/channel/stable/type/iso/file.iso", "", nil)
		require.Equal(t, http.StatusOK, resp.statusCode)
		require.Equal(t, imageContent, body)

		require.Equal(t, before, cachedImages(t, cacheDir, true))
		require.Len(t, cachedImages(t, cacheDir, false), 1)

		public = true
	})
}

// cachedImages lists the images stored in one namespace of the seed image cache.
func cachedImages(t *testing.T, cacheDir string, public bool) []string {
	t.Helper()

	namespace := "private"
	if public {
		namespace = "public"
	}

	entries, err := os.ReadDir(filepath.Join(cacheDir, namespace))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	require.NoError(t, err)

	var names []string

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".img") {
			names = append(names, entry.Name())
		}
	}

	return names
}

// requireSpaceForCachedImage skips the test, if dir cannot take a cached image.
func requireSpaceForCachedImage(t *testing.T, dir string) {
	t.Helper()

	const required = 3 << 30

	var stat unix.Statfs_t

	err := unix.Statfs(dir, &stat)
	require.NoError(t, err)

	available := stat.Bavail * uint64(stat.Bsize)
	if available < required {
		t.Skipf("%q has %d bytes available, the seed image cache requires at least %d", dir, available, required)
	}
}

type readCountingReadCloser struct {
	io.Reader

	reads   *atomic.Int64
	counted bool
}

func (r *readCountingReadCloser) Read(p []byte) (int, error) {
	if !r.counted {
		r.counted = true

		r.reads.Add(1)
	}

	return r.Reader.Read(p)
}

func (r *readCountingReadCloser) Close() error {
	return nil
}
