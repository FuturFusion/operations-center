package redfish

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stmcginnis/gofish/schemas"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

func TestWrapRedfishError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string

		want string
	}{
		{
			// Response of an ASUS ASMB9-iKVM (AMI MegaRAC) rejecting the image
			// URI of a VirtualMedia.InsertMedia action.
			name:       "extended info with message, related properties and resolution",
			statusCode: 400,
			body:       `{"error":{"@Message.ExtendedInfo":[{"@odata.type":"#Message.v1_0_7.Message","Message":"The value https://example.com/file.iso(string) for the property Image is of a different format than the property can accept.","MessageArgs":["https://example.com/file.iso(string)","Image"],"MessageId":"Base.1.5.PropertyValueFormatError","RelatedProperties":["#/Image"],"Resolution":"Correct the value for the property in the request body and resubmit the request if the operation failed.","Severity":"Warning"}],"code":"Base.1.5.PropertyValueFormatError","message":"The value https://example.com/file.iso(string) for the property Image is of a different format than the property can accept."}}`,

			want: "BMC returned HTTP 400: Base.1.5.PropertyValueFormatError: The value https://example.com/file.iso(string) for the property Image is of a different format than the property can accept. (severity: Warning) (related properties: #/Image) Resolution: Correct the value for the property in the request body and resubmit the request if the operation failed.",
		},
		{
			// Response of an HPE iLO rejecting the "Inserted" property of a
			// PATCH against the VirtualMedia resource. The BMC reports the
			// message registry ID only, without a human readable message.
			name:       "extended info with message id and args only",
			statusCode: 400,
			body:       `{"Messages":[{"MessageArgs":["Inserted"],"MessageID":"Base.0.10.PropertyUnknown"}],"Type":"ExtendedError.1.0.0","error":{"@Message.ExtendedInfo":[{"MessageArgs":["Inserted"],"MessageID":"Base.0.10.PropertyUnknown"}],"code":"iLO.0.10.ExtendedInfo","message":"See @Message.ExtendedInfo for more information."}}`,

			want: "BMC returned HTTP 400: Base.0.10.PropertyUnknown [Inserted]",
		},
		{
			name:       "multiple extended info entries",
			statusCode: 400,
			body:       `{"error":{"@Message.ExtendedInfo":[{"MessageId":"Base.1.0.PropertyMissing","MessageArgs":["TransferProtocolType"]},{"MessageId":"Base.1.0.PropertyUnknown","MessageArgs":["TransferMethod"]}],"code":"Base.1.0.GeneralError","message":"A general error has occurred."}}`,

			want: "BMC returned HTTP 400: Base.1.0.PropertyMissing [TransferProtocolType]; Base.1.0.PropertyUnknown [TransferMethod]",
		},
		{
			name:       "no extended info falls back to code and message",
			statusCode: 500,
			body:       `{"error":{"code":"Base.1.0.InternalError","message":"The request failed due to an internal service error."}}`,

			want: "BMC returned HTTP 500: Base.1.0.InternalError: The request failed due to an internal service error.",
		},
		{
			name:       "non redfish body is reported as is",
			statusCode: 503,
			body:       "Service Unavailable",

			want: "BMC returned HTTP 503: Service Unavailable",
		},
		{
			name:       "empty body",
			statusCode: 400,
			body:       "",

			want: "BMC returned HTTP 400: no error details reported",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := wrapRedfishError(schemas.ConstructError(tc.statusCode, []byte(tc.body)))

			require.EqualError(t, err, tc.want)

			// The original error stays reachable, so callers keep the raw
			// response body and the status code.
			var redfishErr *schemas.Error

			require.ErrorAs(t, err, &redfishErr)
			require.Equal(t, tc.statusCode, redfishErr.HTTPReturnedStatusCode)
		})
	}
}

func TestWrapRedfishError_passthrough(t *testing.T) {
	err := errors.New("boom")

	require.Equal(t, err, wrapRedfishError(err))
	require.NoError(t, wrapRedfishError(nil))
}

func TestWrapRedfishError_wrapped(t *testing.T) {
	// A Redfish error nested in a wrapping error is rendered as well.
	err := wrapRedfishError(fmt.Errorf("Failed to attach media: %w", schemas.ConstructError(400, []byte(`{"error":{"@Message.ExtendedInfo":[{"MessageId":"Base.0.10.PropertyUnknown","MessageArgs":["Inserted"]}]}}`))))

	require.EqualError(t, err, "BMC returned HTTP 400: Base.0.10.PropertyUnknown [Inserted]")
}

func TestWrapRedfishError_status(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int

		wantStatus int
		wantMatch  bool
	}{
		{
			// The BMC turned the request down, repeating it unchanged will not
			// help, which the API reports as a bad request.
			name:       "client error",
			statusCode: 400,

			wantStatus: 400,
			wantMatch:  true,
		},
		{
			// The status of the BMC is not the status of the API: the caller of
			// the API is authorized, the credentials for the BMC are not.
			name:       "unauthorized",
			statusCode: 401,

			wantStatus: 400,
			wantMatch:  true,
		},
		{
			// Nothing the caller can do about, so the API keeps reporting an
			// internal error.
			name:       "server error",
			statusCode: 500,

			wantMatch: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := wrapRedfishError(schemas.ConstructError(tc.statusCode, []byte(`{"error":{"code":"Base.1.0.GeneralError","message":"boom"}}`)))

			statusCode, found := api.StatusErrorMatch(err)

			require.Equal(t, tc.wantMatch, found)

			if tc.wantMatch {
				require.Equal(t, tc.wantStatus, statusCode)
			}
		})
	}
}

func TestRedfishRequestError(t *testing.T) {
	err := redfishRequestError(
		schemas.ConstructError(400, []byte(`{"error":{"@Message.ExtendedInfo":[{"MessageID":"Base.0.10.PropertyUnknown","MessageArgs":["Inserted"]}]}}`)),
		nil,
		http.MethodPatch,
		"/redfish/v1/Systems/1/VirtualMedia/1",
		map[string]any{"Image": "http://example.com/install.iso", "Inserted": true},
	)

	require.EqualError(t, err, `PATCH /redfish/v1/Systems/1/VirtualMedia/1 {"Image":"http://example.com/install.iso","Inserted":true}: BMC returned HTTP 400: Base.0.10.PropertyUnknown [Inserted]`)

	// Rendering the error again keeps the request it names.
	require.Equal(t, err, wrapRedfishError(err))

	require.NoError(t, redfishRequestError(nil, nil, http.MethodPatch, "/redfish/v1/Systems/1/VirtualMedia/1", nil))
}

func TestRedactAuthorization(t *testing.T) {
	dump := strings.Join([]string{
		"POST /redfish/v1/Managers/1/VirtualMedia/2 HTTP/1.1",
		"Host: bmc.example.com:8443",
		"authorization: Basic YWRtaW46c2VjcmV0",
		"X-Auth-Token: 7c2a1f",
		"Cookie: sessionKey=deadbeef",
		"Set-Cookie: sessionKey=deadbeef",
		"Content-Type: application/json",
		"",
		`{"Image":"http://example.com/install.iso"}`,
	}, "\r\n")

	want := strings.Join([]string{
		"POST /redfish/v1/Managers/1/VirtualMedia/2 HTTP/1.1",
		"Host: bmc.example.com:8443",
		"Authorization: <redacted>",
		"X-Auth-Token: <redacted>",
		"Cookie: <redacted>",
		"Set-Cookie: <redacted>",
		"Content-Type: application/json",
		"",
		`{"Image":"http://example.com/install.iso"}`,
	}, "\n")

	require.Equal(t, want, redactAuthorization(dump))
}
