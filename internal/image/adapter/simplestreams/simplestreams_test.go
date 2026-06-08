package simplestreams_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	incusSimplestreams "github.com/lxc/incus/v7/shared/simplestreams"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/image/adapter/simplestreams"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestSimplestreams_GetImageList(t *testing.T) {
	type response struct {
		statuscode int
		body       []byte
	}

	tests := []struct {
		name      string
		statusCod int
		responses []queue.Item[response]

		assertErr  require.ErrorAssertionFunc
		wantImages image.IncusImages
	}{
		{
			name: "success - one image",
			responses: []queue.Item[response]{
				{
					Value: response{
						statuscode: http.StatusOK,
						body: func() []byte {
							body, err := json.Marshal(incusSimplestreams.Stream{
								Index: map[string]incusSimplestreams.StreamIndex{
									"images": {
										DataType: "image-downloads",
										Path:     "streams/v1/images.json",
									},
								},
							})
							require.NoError(t, err)
							return body
						}(),
					},
				},
				{
					Value: response{
						statuscode: http.StatusOK,
						body: func() []byte {
							body, err := json.Marshal(incusSimplestreams.Products{
								Products: map[string]incusSimplestreams.Product{
									"alpine:edge:amd64:default": {
										Architecture:    "amd64",
										OperatingSystem: "alpine",
										Release:         "edge",
										Variant:         "default",
										Versions: map[string]incusSimplestreams.ProductVersion{
											"20260615": {
												Items: map[string]incusSimplestreams.ProductVersionItem{
													"incus.tar.xz": {
														Path: "images/alpine/edge/amd64/default/20260615/incus.tar.xz",
													},
												},
											},
										},
									},
								},
							})
							require.NoError(t, err)
							return body
						}(),
					},
				},
			},

			assertErr: require.NoError,
			wantImages: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:default",
					Architecture:    "amd64",
					OperatingSystem: "alpine",
					Release:         "edge",
					Source:          ptr.To("one"),
					Variant:         "default",
					Versions: api.IncusImageVersions{
						"20260615": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {
									Path: "images/alpine/edge/amd64/default/20260615/incus.tar.xz",
								},
							},
						},
					},
				},
			},
		},

		{
			name: "error - index - unexpected status code",
			responses: []queue.Item[response]{
				{
					Value: response{
						statuscode: http.StatusInternalServerError,
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Unexpected status code received for image stream index: 500")
			},
		},
		{
			name: "error - index - invalid JSON",
			responses: []queue.Item[response]{
				{
					Value: response{
						statuscode: http.StatusOK,
						body:       []byte("invalid JSON"),
					},
				},
			},

			assertErr: require.Error,
		},
		{
			name: "error - index - without image-downloads",
			responses: []queue.Item[response]{
				{
					Value: response{
						statuscode: http.StatusOK,
						body: func() []byte {
							body, err := json.Marshal(incusSimplestreams.Stream{
								Index: map[string]incusSimplestreams.StreamIndex{
									"images": {
										DataType: "some data type",
									},
								},
							})
							require.NoError(t, err)
							return body
						}(),
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Image source does not provide data type "image-downloads"`)
			},
		},
		{
			name: "error - images - unexpected status code",
			responses: []queue.Item[response]{
				{
					Value: response{
						statuscode: http.StatusOK,
						body: func() []byte {
							body, err := json.Marshal(incusSimplestreams.Stream{
								Index: map[string]incusSimplestreams.StreamIndex{
									"images": {
										DataType: "image-downloads",
										Path:     "streams/v1/images.json",
									},
								},
							})
							require.NoError(t, err)
							return body
						}(),
					},
				},
				{
					Value: response{
						statuscode: http.StatusInternalServerError,
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Unexpected status code received for image stream image index: 500")
			},
		},
		{
			name: "error - images - unexpected status code",
			responses: []queue.Item[response]{
				{
					Value: response{
						statuscode: http.StatusOK,
						body: func() []byte {
							body, err := json.Marshal(incusSimplestreams.Stream{
								Index: map[string]incusSimplestreams.StreamIndex{
									"images": {
										DataType: "image-downloads",
										Path:     "streams/v1/images.json",
									},
								},
							})
							require.NoError(t, err)
							return body
						}(),
					},
				},
				{
					Value: response{
						statuscode: http.StatusOK,
						body:       []byte("invalid JSON"),
					},
				},
			},

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svr := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					response, _ := queue.Pop(t, &tc.responses)

					w.WriteHeader(response.statuscode)

					_, _ = w.Write(response.body)
				}),
			)
			defer svr.Close()

			simplestreamsAdapter := simplestreams.New()

			imgs, err := simplestreamsAdapter.GetImageList(t.Context(), image.ImageSource{
				Name: "one",
				URL:  svr.URL,
			})
			tc.assertErr(t, err)

			require.Equal(t, tc.wantImages, imgs)
			require.Empty(t, tc.responses)
		})
	}
}

func TestSimplestreams_GetFile(t *testing.T) {
	tests := []struct {
		name         string
		argFileName  string
		statusCode   int
		responseBody []byte

		assertErr        require.ErrorAssertionFunc
		wantFilePath     string
		wantResponseBody []byte
	}{
		{
			name:         "success - relative path",
			argFileName:  "some/dir/incus.tar.xz",
			statusCode:   http.StatusOK,
			responseBody: []byte(`some text`),

			assertErr:        require.NoError,
			wantFilePath:     "/streams/v1/some/dir/incus.tar.xz",
			wantResponseBody: []byte(`some text`),
		},
		{
			name:         "success - absolute path",
			argFileName:  "/some/dir/incus.tar.xz",
			statusCode:   http.StatusOK,
			responseBody: []byte(`some text`),

			assertErr:        require.NoError,
			wantFilePath:     "/some/dir/incus.tar.xz",
			wantResponseBody: []byte(`some text`),
		},
		{
			name:        "error - wrong status code",
			argFileName: "some/dir/incus.tar.xz",
			statusCode:  http.StatusInternalServerError,

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svr := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != tc.wantFilePath {
						w.WriteHeader(http.StatusNotFound)
						return
					}

					w.WriteHeader(tc.statusCode)
					_, _ = w.Write(tc.responseBody)
				}),
			)
			defer svr.Close()

			simplestreamsAdapter := simplestreams.New()

			rc, err := simplestreamsAdapter.GetFile(t.Context(), image.ImageSource{
				Name: "one",
				URL:  svr.URL + "/streams/v1",
			}, tc.argFileName)
			tc.assertErr(t, err)

			responseBody := readAll(t, rc)

			require.Equal(t, tc.wantResponseBody, responseBody)
		})
	}
}

func readAll(t *testing.T, r io.ReadCloser) []byte {
	t.Helper()

	if r == nil {
		return nil
	}

	body, err := io.ReadAll(r)
	require.NoError(t, err)

	return body
}
