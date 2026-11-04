package redfish

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_recordedHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header http.Header

		want map[string][]string
	}{
		{
			name:   "nil",
			header: nil,

			want: nil,
		},
		{
			name: "credentials are redacted, other headers are copied",
			header: http.Header{
				"Authorization": {"Basic YWRtaW46c2VjcmV0"},
				"X-Auth-Token":  {"token"},
				"cookie":        {"session=1"},
				"Set-Cookie":    {"session=1", "other=2"},
				"Content-Type":  {"application/json"},
				"Allow":         {"GET", "PATCH"},
			},

			want: map[string][]string{
				"Authorization": {"<redacted>"},
				"X-Auth-Token":  {"<redacted>"},
				"cookie":        {"<redacted>"},
				"Set-Cookie":    {"<redacted>"},
				"Content-Type":  {"application/json"},
				"Allow":         {"GET", "PATCH"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := recordedHeaders(tc.header)

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_recordedHeaders_copy(t *testing.T) {
	t.Parallel()

	header := http.Header{"Allow": {"GET"}}

	got := recordedHeaders(header)
	header["Allow"][0] = "PATCH"

	require.Equal(t, []string{"GET"}, got["Allow"], "the recorded headers do not share memory with the request")
}

func Test_recordedBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		data      []byte
		truncated bool

		wantBody     string
		wantEncoding string
	}{
		{
			name: "empty",
			data: nil,

			wantBody:     "",
			wantEncoding: "",
		},
		{
			name: "text",
			data: []byte(`{"Id":"é"}`),

			wantBody:     `{"Id":"é"}`,
			wantEncoding: "",
		},
		{
			name: "binary",
			data: []byte{0xff, 0xfe, 0x00},

			wantBody:     "//4A",
			wantEncoding: api.BMCRecordingBodyEncodingBase64,
		},
		{
			name: "complete body ending in an incomplete character is binary",
			data: []byte("abc\xc3"),

			wantBody:     "YWJjww==",
			wantEncoding: api.BMCRecordingBodyEncodingBase64,
		},
		{
			name:      "truncated text cut inside a character",
			data:      []byte("abc\xe2\x82"),
			truncated: true,

			wantBody:     "abc",
			wantEncoding: "",
		},
		{
			name:      "truncated text cut after a complete character",
			data:      []byte("abc€"),
			truncated: true,

			wantBody:     "abc€",
			wantEncoding: "",
		},
		{
			name:      "truncated binary",
			data:      []byte{0xff, 0xfe, 0xfd, 0xfc, 0xfb},
			truncated: true,

			wantBody:     "//79/Ps=",
			wantEncoding: api.BMCRecordingBodyEncodingBase64,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body, encoding := recordedBody(tc.data, tc.truncated)

			require.Equal(t, tc.wantBody, body)
			require.Equal(t, tc.wantEncoding, encoding)
		})
	}
}

func Test_redactJSONKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string

		want string
	}{
		{
			name: "top level",
			data: `{"UserName":"admin","Password":"secret"}`,

			want: `{"Password":"<redacted>","UserName":"admin"}`,
		},
		{
			name: "nested object",
			data: `{"Oem":{"Vendor":{"Token":"abc"}}}`,

			want: `{"Oem":{"Vendor":{"Token":"<redacted>"}}}`,
		},
		{
			name: "array of objects",
			data: `[{"password":"one"},{"Name":"two"}]`,

			want: `[{"password":"<redacted>"},{"Name":"two"}]`,
		},
		{
			name: "non string value stays",
			data: `{"Password":null,"Token":{"Id":"1"}}`,

			want: `{"Password":null,"Token":{"Id":"1"}}`,
		},
		{
			name: "no match returns the original bytes",
			data: "{ \"Name\": \"x\", \"Count\": 12345678901234567890 }",

			want: "{ \"Name\": \"x\", \"Count\": 12345678901234567890 }",
		},
		{
			name: "numbers keep their precision",
			data: `{"Count":12345678901234567890,"Password":"secret"}`,

			want: `{"Count":12345678901234567890,"Password":"<redacted>"}`,
		},
		{
			name: "not JSON",
			data: `Password=secret`,

			want: `Password=secret`,
		},
		{
			name: "truncated JSON",
			data: `{"Password":"sec`,

			want: `{"Password":"sec`,
		},
		{
			name: "trailing data",
			data: `{"Password":"secret"} {}`,

			want: `{"Password":"secret"} {}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := redactJSONKeys([]byte(tc.data), redactedJSONKeys...)

			require.Equal(t, tc.want, string(got))
		})
	}
}

type fakeBMCRecorder struct {
	ctx       context.Context
	exchanges []api.BMCRecordingExchange
}

func (f *fakeBMCRecorder) Record(ctx context.Context, exchange api.BMCRecordingExchange) {
	f.ctx = ctx
	f.exchanges = append(f.exchanges, exchange)
}

func (f *fakeBMCRecorder) Stats() provisioning.BMCRecordingStats {
	return provisioning.BMCRecordingStats{Exchanges: int64(len(f.exchanges))}
}

type testContextKey struct{}

func Test_newExchangeObserver(t *testing.T) {
	t.Parallel()

	recorder := &fakeBMCRecorder{}
	ctx := context.WithValue(t.Context(), testContextKey{}, "call")

	observe := newExchangeObserver(ctx, provisioning.BMCRecordingScope{
		Recorder: recorder,
		State:    api.ServerDeploymentStateApplyBIOS,
		Retries:  2,
	})

	observe(api.BMCRecordingExchange{Method: http.MethodGet, Attempt: 1})

	require.Equal(t, []api.BMCRecordingExchange{
		{
			DeploymentState: api.ServerDeploymentStateApplyBIOS,
			StepRetries:     2,
			Method:          http.MethodGet,
			Attempt:         1,
		},
	}, recorder.exchanges)
	require.Equal(t, "call", recorder.ctx.Value(testContextKey{}), "the recorder gets the context of the adapter call")
}
