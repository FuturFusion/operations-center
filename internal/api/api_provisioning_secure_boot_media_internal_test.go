package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/securebootcerts"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/securebootmedia"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_secureBootMediaGet(t *testing.T) {
	for _, imageType := range []api.ImageType{api.ImageTypeISO, api.ImageTypeRaw} {
		t.Run(imageType.String(), func(t *testing.T) {
			err := securebootmedia.New(t.TempDir()).CheckSupported(t.Context(), imageType, images.UpdateFileArchitecture64BitX86)
			if err != nil {
				t.Skipf("The secure boot enrollment media can not be generated here: %v", err)
			}

			catalog, err := securebootcerts.New()
			require.NoError(t, err)

			certificates, unknown := catalog.CertificatesByFingerprint(catalog.Fingerprints())
			require.Empty(t, unknown)

			dir := filepath.Join(t.TempDir(), "secure-boot-media")
			media := securebootmedia.New(dir)

			id, err := media.Generate(t.Context(), imageType, images.UpdateFileArchitecture64BitX86, provisioning.SecureBootCertificates{
				PK:  certificates[0],
				KEK: []string{certificates[1]},
				DB:  []string{certificates[2]},
			})
			require.NoError(t, err)

			image, err := os.ReadFile(filepath.Join(dir, id+imageType.FileExt()))
			require.NoError(t, err)

			serveMux := http.NewServeMux()
			registerSecureBootMediaHandler(newRouter(serveMux).SubGroup("/1.0/provisioning/secure-boot-media"), media)

			server := httptest.NewServer(serveMux)
			t.Cleanup(server.Close)

			path := "/" + filepath.Join(api.SecureBootMediaPathSegments(imageType, id)...)

			t.Run("serves the generated media", func(t *testing.T) {
				body, resp := doSecureBootMediaRequest(t, server, path, nil)

				require.Equal(t, http.StatusOK, resp.statusCode)
				require.Equal(t, image, body, "the media is served exactly as it was generated")
				require.Equal(t, "bytes", resp.header.Get("Accept-Ranges"), "a BMC has to be able to resume an interrupted transfer")
			})

			t.Run("serves a byte range", func(t *testing.T) {
				body, resp := doSecureBootMediaRequest(t, server, path, http.Header{"Range": []string{"bytes=2048-4095"}})

				require.Equal(t, http.StatusPartialContent, resp.statusCode)
				require.Equal(t, image[2048:4096], body)
			})

			t.Run("rejects a filename without the extension of an image type", func(t *testing.T) {
				for _, filename := range []string{id, id + ".img"} {
					_, resp := doSecureBootMediaRequest(t, server, "/1.0/provisioning/secure-boot-media/"+filename, nil)

					require.Equal(t, http.StatusBadRequest, resp.statusCode, "%q does not address a media", filename)
				}
			})

			t.Run("reports a media of another image type as not found", func(t *testing.T) {
				for _, other := range []api.ImageType{api.ImageTypeISO, api.ImageTypeRaw} {
					if other == imageType {
						continue
					}

					_, resp := doSecureBootMediaRequest(t, server, "/1.0/provisioning/secure-boot-media/"+id+other.FileExt(), nil)

					require.Equal(t, http.StatusNotFound, resp.statusCode, "the media has only been generated as %q", imageType)
				}
			})

			t.Run("reports a media, that has not been generated, as not found", func(t *testing.T) {
				_, resp := doSecureBootMediaRequest(t, server, "/1.0/provisioning/secure-boot-media/AAAAAAAAAAAA"+imageType.FileExt(), nil)

				require.Equal(t, http.StatusNotFound, resp.statusCode)
			})

			t.Run("reports an ID, that can not address a media, as not found", func(t *testing.T) {
				_, resp := doSecureBootMediaRequest(t, server, "/1.0/provisioning/secure-boot-media/..%2f..%2fetc%2fpasswd"+imageType.FileExt(), nil)

				require.Equal(t, http.StatusNotFound, resp.statusCode)
			})
		})
	}
}

// secureBootMediaResponse holds what the assertions look at, so the response
// itself never leaves the helper, which closes it.
type secureBootMediaResponse struct {
	statusCode int
	header     http.Header
}

func doSecureBootMediaRequest(t *testing.T, server *httptest.Server, target string, header http.Header) ([]byte, secureBootMediaResponse) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+target, http.NoBody)
	require.NoError(t, err)

	for name, values := range header {
		req.Header[name] = values
	}

	resp, err := server.Client().Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return body, secureBootMediaResponse{
		statusCode: resp.StatusCode,
		header:     resp.Header,
	}
}
