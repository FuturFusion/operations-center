package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/seedprogress"
	provisioningMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/shared/api"
)

const tokenUUID = "b32d0079-c48b-4957-b1cb-bef54125c861"

func Test_tokenHandler_tokenSeedsPost(t *testing.T) {
	tests := []struct {
		name        string
		requestBody string

		wantStatus              int
		wantResponseBodyContain string
		wantServiceCalled       bool
	}{
		{
			name: "success",
			requestBody: `{
  "name": "test",
  "description": "some description",
  "public": false,
  "seeds": {
    "network": {
      "version": "1"
    }
  }
}`,

			wantStatus:        http.StatusCreated,
			wantServiceCalled: true,
		},
		{
			name: "error - network seed nested in config block",
			requestBody: `{
  "name": "test",
  "seeds": {
    "network": {
      "config": {}
    }
  }
}`,

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: `unknown field \"config\"`,
		},
		{
			name: "error - fields only used by the UI form",
			requestBody: `{
  "name": "test",
  "seeds": {
    "application": "incus",
    "secondary_applications": []
  }
}`,

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: `unknown field \"application\"`,
		},
		{
			name: "error - incus seed is managed by operations center",
			requestBody: `{
  "name": "test",
  "seeds": {
    "incus": {
      "version": "1"
    }
  }
}`,

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: `unknown field \"incus\"`,
		},
		{
			name: "error - update seed is managed by operations center",
			requestBody: `{
  "name": "test",
  "seeds": {
    "update": {
      "version": "1"
    }
  }
}`,

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: `unknown field \"update\"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var serviceCalled bool

			tokenService := &provisioningMock.TokenServiceMock{
				CreateTokenSeedFunc: func(ctx context.Context, tokenSeedConfig provisioning.TokenSeed) (provisioning.TokenSeed, error) {
					serviceCalled = true

					return tokenSeedConfig, nil
				},
			}

			body, status := doTokenRequest(t, tokenService, http.MethodPost, "/"+tokenUUID+"/seeds", tc.requestBody)

			require.Equal(t, tc.wantStatus, status)
			require.Equal(t, tc.wantServiceCalled, serviceCalled)

			if tc.wantResponseBodyContain != "" {
				require.Contains(t, body, tc.wantResponseBodyContain)
			}
		})
	}
}

func Test_tokenHandler_tokenSeedPut(t *testing.T) {
	tests := []struct {
		name        string
		requestBody string

		wantStatus              int
		wantResponseBodyContain string
	}{
		{
			name: "error - network seed nested in config block",
			requestBody: `{
  "description": "some description",
  "seeds": {
    "network": {
      "config": {}
    }
  }
}`,

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: `unknown field \"config\"`,
		},
		{
			name:        "error - name is not part of the update request",
			requestBody: `{"name": "test", "description": "some description"}`,

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: `unknown field \"name\"`,
		},
		{
			name: "error - incus seed is managed by operations center",
			requestBody: `{
  "seeds": {
    "incus": {
      "version": "1"
    }
  }
}`,

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: `unknown field \"incus\"`,
		},
		{
			name: "error - update seed is managed by operations center",
			requestBody: `{
  "seeds": {
    "update": {
      "version": "1"
    }
  }
}`,

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: `unknown field \"update\"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var serviceCalled bool

			tokenService := &provisioningMock.TokenServiceMock{
				GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
					serviceCalled = true

					return &provisioning.TokenSeed{}, nil
				},
			}

			body, status := doTokenRequest(t, tokenService, http.MethodPut, "/"+tokenUUID+"/seeds/test", tc.requestBody)

			require.Equal(t, tc.wantStatus, status)
			require.False(t, serviceCalled)
			require.Contains(t, body, tc.wantResponseBodyContain)
		})
	}
}

func Test_tokenHandler_tokenSeedGet(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{
			name: "plain",
			path: "/" + tokenUUID + "/seeds/test",
		},
		{
			// The image used to be served from this route, if the "type" query
			// parameter was set. It is served from
			// GET /{uuid}/seeds/{name}/{params...} exclusively now, so the
			// query parameters are without effect.
			name: "image query parameters are ignored",
			path: "/" + tokenUUID + "/seeds/test?type=iso&architecture=x86_64&channel=stable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var imageServiceCalled bool

			tokenService := &provisioningMock.TokenServiceMock{
				GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
					return &provisioning.TokenSeed{
						Token:       uuid.MustParse(tokenUUID),
						Name:        name,
						Description: "test seed",
					}, nil
				},
				GetCompressedTokenImageFromTokenSeedFunc: func(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (io.ReadCloser, error) {
					imageServiceCalled = true

					return io.NopCloser(strings.NewReader("image-data")), nil
				},
				GetSeekableTokenImageFromTokenSeedFunc: func(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (*provisioning.TokenImage, error) {
					imageServiceCalled = true

					return testTokenImage("image-data"), nil
				},
			}

			body, resp := doTokenRequestFull(t, tokenService, http.MethodGet, tc.path, "", nil)

			require.Equal(t, http.StatusOK, resp.statusCode)
			require.False(t, imageServiceCalled)
			require.Contains(t, resp.header.Get("Content-Type"), "application/json")
			require.Contains(t, body, `"name":"test"`)
			require.Contains(t, body, `"description":"test seed"`)
		})
	}
}

func Test_tokenHandler_tokenSeedImageGet(t *testing.T) {
	tests := []struct {
		name string
		path string

		wantStatus        int
		wantServiceCalled bool
		wantImageType     api.ImageType
		wantArchitecture  images.UpdateFileArchitecture
		wantChannel       string
	}{
		{
			name: "success - iso, no channel",
			path: "/" + tokenUUID + "/seeds/test/architecture/x86_64/type/iso/file.iso",

			wantStatus:        http.StatusOK,
			wantServiceCalled: true,
			wantImageType:     api.ImageTypeISO,
			wantArchitecture:  images.UpdateFileArchitecture64BitX86,
		},
		{
			name: "success - raw, with channel, keys reordered",
			path: "/" + tokenUUID + "/seeds/test/type/raw/channel/stable/architecture/aarch64/whatever-not-inspected",

			wantStatus:        http.StatusOK,
			wantServiceCalled: true,
			wantImageType:     api.ImageTypeRaw,
			wantArchitecture:  images.UpdateFileArchitecture64BitARM,
			wantChannel:       "stable",
		},
		{
			name: "success - trailing filename with no extension is accepted and ignored",
			path: "/" + tokenUUID + "/seeds/test/architecture/x86_64/type/iso/noextension",

			wantStatus:        http.StatusOK,
			wantServiceCalled: true,
			wantImageType:     api.ImageTypeISO,
			wantArchitecture:  images.UpdateFileArchitecture64BitX86,
		},
		{
			name: "error - missing type",
			path: "/" + tokenUUID + "/seeds/test/architecture/x86_64/file.iso",

			wantStatus: http.StatusBadRequest,
		},
		{
			name: "error - missing architecture",
			path: "/" + tokenUUID + "/seeds/test/type/iso/file.iso",

			wantStatus: http.StatusBadRequest,
		},
		{
			name: "error - invalid type",
			path: "/" + tokenUUID + "/seeds/test/architecture/x86_64/type/exe/file.exe",

			wantStatus: http.StatusBadRequest,
		},
		{
			name: "error - invalid architecture",
			path: "/" + tokenUUID + "/seeds/test/architecture/mips/type/iso/file.iso",

			wantStatus: http.StatusBadRequest,
		},
		{
			name: "error - unknown key",
			path: "/" + tokenUUID + "/seeds/test/architecture/x86_64/type/iso/bogus/value/file.iso",

			wantStatus: http.StatusBadRequest,
		},
		{
			name: "error - odd number of key/value segments",
			path: "/" + tokenUUID + "/seeds/test/architecture/x86_64/type/file.iso",

			wantStatus: http.StatusBadRequest,
		},
		{
			name: "error - missing filename segment entirely",
			path: "/" + tokenUUID + "/seeds/test/",

			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var serviceCalled bool

			var gotImageType api.ImageType

			var gotArchitecture images.UpdateFileArchitecture

			var gotChannel string

			tokenService := &provisioningMock.TokenServiceMock{
				GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
					return &provisioning.TokenSeed{Public: true}, nil
				},
				GetSeekableTokenImageFromTokenSeedFunc: func(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (*provisioning.TokenImage, error) {
					serviceCalled = true
					gotImageType = imageType
					gotArchitecture = architecture
					gotChannel = channel

					return testTokenImage("image-data"), nil
				},
			}

			body, resp := doTokenRequestFull(t, tokenService, http.MethodGet, tc.path, "", nil)

			require.Equal(t, tc.wantStatus, resp.statusCode)
			require.Equal(t, tc.wantServiceCalled, serviceCalled)

			if tc.wantServiceCalled {
				require.Equal(t, tc.wantImageType, gotImageType)
				require.Equal(t, tc.wantArchitecture, gotArchitecture)
				require.Equal(t, tc.wantChannel, gotChannel)
				require.Equal(t, "image-data", body)
			}
		})
	}
}

func Test_tokenHandler_tokenSeedImageGet_cached(t *testing.T) {
	const (
		path      = "/" + tokenUUID + "/seeds/test/architecture/x86_64/type/iso/file.iso"
		imageData = "cached-image-data"
	)

	tests := []struct {
		name          string
		requestHeader http.Header

		wantCached bool
	}{
		{
			name: "a plain request is served from the cache",

			wantCached: true,
		},
		{
			name:          "a range request is served from the cache",
			requestHeader: http.Header{"Range": []string{"bytes=7-11"}},

			wantCached: true,
		},
		{
			name:          "a request for the compressed file is streamed, not cached",
			requestHeader: http.Header{"Accept": []string{"application/gzip"}},

			wantCached: false,
		},
		{
			name: "a range wins over a request for the compressed file and is served from the cache",
			requestHeader: http.Header{
				"Accept": []string{"application/gzip"},
				"Range":  []string{"bytes=7-11"},
			},

			wantCached: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var cachedCalled bool

			tokenService := &provisioningMock.TokenServiceMock{
				GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
					return &provisioning.TokenSeed{Public: true}, nil
				},
				GetSeekableTokenImageFromTokenSeedFunc: func(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (*provisioning.TokenImage, error) {
					cachedCalled = true

					return testTokenImage(imageData), nil
				},
				GetCompressedTokenImageFromTokenSeedFunc: func(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader(imageData)), nil
				},
			}

			body, resp := doTokenRequestFull(t, tokenService, http.MethodGet, path, "", tc.requestHeader)

			require.Equal(t, tc.wantCached, cachedCalled)

			switch {
			case tc.requestHeader.Get("Range") != "":
				require.Equal(t, http.StatusPartialContent, resp.statusCode)
				require.Equal(t, imageData[7:12], body)

			case tc.requestHeader.Get("Accept") == "application/gzip":
				require.Equal(t, http.StatusOK, resp.statusCode)
				require.Equal(t, imageData, gunzip(t, body))

			default:
				require.Equal(t, http.StatusOK, resp.statusCode)
				require.Equal(t, imageData, body)
			}
		})
	}
}

func Test_tokenHandler_tokenSeedImageGet_wireFormat(t *testing.T) {
	const (
		path      = "/" + tokenUUID + "/seeds/test/architecture/x86_64/type/iso/file.iso"
		imageData = "image-data"
	)

	newTokenService := func() *provisioningMock.TokenServiceMock {
		return &provisioningMock.TokenServiceMock{
			GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
				return &provisioning.TokenSeed{Public: true}, nil
			},
			GetCompressedTokenImageFromTokenSeedFunc: func(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewBufferString(imageData)), nil
			},
			GetSeekableTokenImageFromTokenSeedFunc: func(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (*provisioning.TokenImage, error) {
				return testTokenImage(imageData), nil
			},
		}
	}

	t.Run("client without accept-encoding gets the image uncompressed", func(t *testing.T) {
		body, resp := doTokenRequestFull(t, newTokenService(), http.MethodGet, path, "", nil)

		require.Equal(t, http.StatusOK, resp.statusCode)
		require.Equal(t, imageData, body)
		require.Equal(t, "application/octet-stream", resp.header.Get("Content-Type"))
		require.Empty(t, resp.header.Get("Content-Encoding"))
		require.Equal(t, `attachment; filename="pre-seed-test.iso"`, resp.header.Get("Content-Disposition"))
		require.Equal(t, int64(len(imageData)), resp.contentLength)
		require.Equal(t, "bytes", resp.header.Get("Accept-Ranges"))
	})

	t.Run("client accepting gzip still gets the rangeable image uncompressed", func(t *testing.T) {
		body, resp := doTokenRequestFull(t, newTokenService(), http.MethodGet, path, "", http.Header{"Accept-Encoding": []string{"gzip"}})

		require.Equal(t, http.StatusOK, resp.statusCode)
		require.Equal(t, imageData, body)
		require.Empty(t, resp.header.Get("Content-Encoding"))
		require.Equal(t, "application/octet-stream", resp.header.Get("Content-Type"))
		require.Equal(t, `attachment; filename="pre-seed-test.iso"`, resp.header.Get("Content-Disposition"))
		require.Equal(t, int64(len(imageData)), resp.contentLength)
		require.Equal(t, "bytes", resp.header.Get("Accept-Ranges"))
	})

	t.Run("client asking for the gzip file gets it compressed", func(t *testing.T) {
		body, resp := doTokenRequestFull(t, newTokenService(), http.MethodGet, path, "", http.Header{"Accept": []string{"application/gzip"}})

		require.Equal(t, http.StatusOK, resp.statusCode)
		require.Equal(t, "application/gzip", resp.header.Get("Content-Type"))
		require.Equal(t, `attachment; filename="pre-seed-test.iso.gz"`, resp.header.Get("Content-Disposition"))
		require.Equal(t, "none", resp.header.Get("Accept-Ranges"))
		require.Empty(t, resp.header.Get("Content-Encoding"))

		require.Equal(t, int64(len(body)), resp.contentLength)

		gzipReader, err := gzip.NewReader(strings.NewReader(body))
		require.NoError(t, err)

		decompressed, err := io.ReadAll(gzipReader)
		require.NoError(t, err)
		require.Equal(t, imageData, string(decompressed))
	})

	t.Run("head reports the metadata without a body", func(t *testing.T) {
		body, resp := doTokenRequestFull(t, newTokenService(), http.MethodHead, path, "", nil)

		require.Equal(t, http.StatusOK, resp.statusCode)
		require.Empty(t, body)
		require.Equal(t, "application/octet-stream", resp.header.Get("Content-Type"))
		require.Equal(t, int64(len(imageData)), resp.contentLength)
	})
}

func testTokenImage(content string) *provisioning.TokenImage {
	return &provisioning.TokenImage{
		Content: nopReadSeekCloser{strings.NewReader(content)},
		SeedImageInfo: provisioning.SeedImageInfo{
			Size:    int64(len(content)),
			ModTime: testTokenImageModTime,
		},
		Filename: "pre-seed-test.iso",
	}
}

var testTokenImageModTime = time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

type nopReadSeekCloser struct {
	io.ReadSeeker
}

func (nopReadSeekCloser) Close() error { return nil }

func Test_tokenHandler_tokenSeedImageGetRange(t *testing.T) {
	const content = "0123456789abcdefghij"

	const path = "/" + tokenUUID + "/seeds/test/architecture/x86_64/type/iso/file.iso"

	tests := []struct {
		name          string
		requestHeader http.Header

		wantStatus      int
		wantBody        string
		wantHeader      map[string]string
		wantMissingHead []string
	}{
		{
			name: "no range - full content with a content length",

			wantStatus: http.StatusOK,
			wantBody:   content,
			wantHeader: map[string]string{
				"Accept-Ranges":  "bytes",
				"Content-Length": strconv.Itoa(len(content)),
				"Content-Type":   "application/octet-stream",
			},
			wantMissingHead: []string{"Content-Encoding"},
		},
		{
			name:          "range - a section in the middle",
			requestHeader: http.Header{"Range": []string{"bytes=5-9"}},

			wantStatus: http.StatusPartialContent,
			wantBody:   content[5:10],
			wantHeader: map[string]string{
				"Content-Range":  "bytes 5-9/20",
				"Content-Length": "5",
			},
		},
		{
			name:          "range - open ended, as a resuming client sends it",
			requestHeader: http.Header{"Range": []string{"bytes=12-"}},

			wantStatus: http.StatusPartialContent,
			wantBody:   content[12:],
			wantHeader: map[string]string{
				"Content-Range": "bytes 12-19/20",
			},
		},
		{
			name:          "range - beyond the end of the content",
			requestHeader: http.Header{"Range": []string{"bytes=100-200"}},

			wantStatus: http.StatusRequestedRangeNotSatisfiable,
			wantHeader: map[string]string{
				"Content-Range": "bytes */20",
			},
		},
		{
			name: "range - resumed after the content changed",
			requestHeader: http.Header{
				"Range":    []string{"bytes=12-"},
				"If-Range": []string{testTokenImageModTime.Add(-time.Hour).UTC().Format(http.TimeFormat)},
			},

			wantStatus: http.StatusOK,
			wantBody:   content,
		},
		{
			name: "range - resumed while the content is unchanged",
			requestHeader: http.Header{
				"Range":    []string{"bytes=12-"},
				"If-Range": []string{testTokenImageModTime.UTC().Format(http.TimeFormat)},
			},

			wantStatus: http.StatusPartialContent,
			wantBody:   content[12:],
		},
		{
			name: "range - wins over a request for the compressed file",
			requestHeader: http.Header{
				"Range":  []string{"bytes=5-9"},
				"Accept": []string{"application/gzip"},
			},

			wantStatus: http.StatusPartialContent,
			wantBody:   content[5:10],
			wantHeader: map[string]string{
				"Content-Range": "bytes 5-9/20",
			},
			wantMissingHead: []string{"Content-Encoding"},
		},
		{
			name: "range - offering a compressed transfer does not cost range support",
			requestHeader: http.Header{
				"Range":           []string{"bytes=5-9"},
				"Accept-Encoding": []string{"gzip"},
			},

			wantStatus: http.StatusPartialContent,
			wantBody:   content[5:10],
			wantHeader: map[string]string{
				"Content-Range": "bytes 5-9/20",
			},
			wantMissingHead: []string{"Content-Encoding"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tokenService := &provisioningMock.TokenServiceMock{
				GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
					return &provisioning.TokenSeed{Public: true}, nil
				},
				GetSeekableTokenImageFromTokenSeedFunc: func(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (*provisioning.TokenImage, error) {
					return testTokenImage(content), nil
				},
			}

			body, resp := doTokenRequestFull(t, tokenService, http.MethodGet, path, "", tc.requestHeader)

			require.Equal(t, tc.wantStatus, resp.statusCode)

			if tc.wantStatus != http.StatusRequestedRangeNotSatisfiable {
				require.Equal(t, tc.wantBody, body)
			}

			for k, v := range tc.wantHeader {
				require.Equal(t, v, resp.header.Get(k), "header %q", k)
			}

			for _, k := range tc.wantMissingHead {
				require.Empty(t, resp.header.Get(k), "header %q", k)
			}
		})
	}
}

func Test_tokenHandler_tokenSeedImageGet_readProgress(t *testing.T) {
	const (
		fingerprintID = "AAAAAAAAAAAA"
		path          = "/" + tokenUUID + "/seeds/test/architecture/x86_64/channel/stable/type/iso/" + fingerprintID + ".iso"
		content       = "0123456789abcdefghij"
	)

	tokenService := &provisioningMock.TokenServiceMock{
		GetPreparedTokenSeedImageFunc: func(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string, fingerprintID string) (*provisioning.TokenImage, error) {
			return testTokenImage(content), nil
		},
		GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
			return &provisioning.TokenSeed{Public: true}, nil
		},
		GetSeekableTokenImageFromTokenSeedFunc: func(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (*provisioning.TokenImage, error) {
			return testTokenImage(content), nil
		},
	}

	tracker := seedprogress.New()
	server := newTokenTestServer(t, tokenService, tracker)

	imageID := provisioning.SeedImageID{
		CacheID:       provisioning.SeedImageCacheID(uuid.MustParse(tokenUUID), "test", api.ImageTypeISO, images.UpdateFileArchitecture64BitX86, "stable"),
		FingerprintID: fingerprintID,
	}

	// A BMC streams the image from its own address, which is what tells the
	// reads of several servers installing from the same image apart.
	firstBMC := loopbackClient(t, "127.0.0.1")
	secondBMC := loopbackClient(t, "127.0.0.2")

	body, resp := doTokenRequestFullWithClient(t, firstBMC, server, http.MethodGet, path, "", nil)
	require.Equal(t, http.StatusOK, resp.statusCode)
	require.Equal(t, content, body)

	body, resp = doTokenRequestFullWithClient(t, firstBMC, server, http.MethodGet, path, "", http.Header{"Range": []string{"bytes=5-9"}})
	require.Equal(t, http.StatusPartialContent, resp.statusCode)
	require.Equal(t, content[5:10], body)

	body, resp = doTokenRequestFullWithClient(t, secondBMC, server, http.MethodGet, path, "", http.Header{"Range": []string{"bytes=10-14"}})
	require.Equal(t, http.StatusPartialContent, resp.statusCode)
	require.Equal(t, content[10:15], body)

	firstProgress, ok := tracker.Get(context.Background(), imageID, "127.0.0.1")
	require.True(t, ok)
	require.Equal(t, int64(len(content)), firstProgress.Size)

	require.Equal(t, int64(len(content)+5), firstProgress.BytesServed)
	require.Equal(t, int64(len(content)), firstProgress.BytesCovered, "the range re-read after the full download is only covered once")
	require.Equal(t, 2, firstProgress.RequestCount)

	require.False(t, firstProgress.FirstRead.IsZero())
	require.Equal(t, time.Minute, firstProgress.IdleFor(firstProgress.LastRead.Add(time.Minute)))

	secondProgress, ok := tracker.Get(context.Background(), imageID, "127.0.0.2")
	require.True(t, ok)
	require.Equal(t, int64(5), secondProgress.BytesServed)
	require.Equal(t, int64(5), secondProgress.BytesCovered, "a range request is attributed to the part of the image it names")
	require.Equal(t, 1, secondProgress.RequestCount)

	// A download naming the token seed instead of a prepared image is served to
	// a CLI or to the UI and is not tracked at all.
	downloadPath := "/" + tokenUUID + "/seeds/test/architecture/x86_64/channel/stable/type/iso/file.iso"

	body, resp = doTokenRequestFullWithClient(t, loopbackClient(t, "127.0.0.3"), server, http.MethodGet, downloadPath, "", nil)
	require.Equal(t, http.StatusOK, resp.statusCode)
	require.Equal(t, content, body)

	_, ok = tracker.Get(context.Background(), imageID, "127.0.0.3")
	require.False(t, ok)

	_, ok = tracker.Get(context.Background(), provisioning.SeedImageID{CacheID: imageID.CacheID}, "127.0.0.3")
	require.False(t, ok)
}

func loopbackClient(t *testing.T, address string) *http.Client {
	t.Helper()

	dialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(address),
		},
	}

	client := &http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
			DialContext:        dialer.DialContext,
		},
	}

	t.Cleanup(client.CloseIdleConnections)

	return client
}

func doTokenRequest(t *testing.T, tokenService provisioning.TokenService, method string, target string, requestBody string) (string, int) {
	t.Helper()

	body, resp := doTokenRequestFull(t, tokenService, method, target, requestBody, nil)

	return body, resp.statusCode
}

type tokenResponse struct {
	statusCode    int
	header        http.Header
	contentLength int64
}

func doTokenRequestFull(t *testing.T, tokenService provisioning.TokenService, method string, target string, requestBody string, requestHeader http.Header) (string, tokenResponse) {
	t.Helper()

	server := newTokenTestServer(t, tokenService, seedprogress.New())

	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	t.Cleanup(client.CloseIdleConnections)

	return doTokenRequestFullWithClient(t, client, server, method, target, requestBody, requestHeader)
}

func newTokenTestServer(t *testing.T, tokenService provisioning.TokenService, seedProgress provisioning.SeedImageProgressPort) *httptest.Server {
	t.Helper()

	authenticator := authn.New([]authn.Auther{dummyAuthenticator{}})

	serveMux := http.NewServeMux()
	router := newRouter(serveMux).AddMiddlewares(
		authenticator.Middleware(),
	)

	var authorizer authz.Authorizer = noopAuthorizer{}
	registerProvisioningTokenHandler(router, &authorizer, tokenService, seedProgress)

	server := httptest.NewServer(serveMux)
	t.Cleanup(server.Close)

	return server
}

func doTokenRequestFullWithClient(t *testing.T, client *http.Client, server *httptest.Server, method string, target string, requestBody string, requestHeader http.Header) (string, tokenResponse) {
	t.Helper()

	req, err := http.NewRequest(method, server.URL+target, bytes.NewBufferString(requestBody))
	require.NoError(t, err)

	maps.Copy(req.Header, requestHeader)

	resp, err := client.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return string(body), tokenResponse{
		statusCode:    resp.StatusCode,
		header:        resp.Header,
		contentLength: resp.ContentLength,
	}
}

func gunzip(t *testing.T, body string) string {
	t.Helper()

	gzipReader, err := gzip.NewReader(strings.NewReader(body))
	require.NoError(t, err)

	decompressed, err := io.ReadAll(gzipReader)
	require.NoError(t, err)

	return string(decompressed)
}
