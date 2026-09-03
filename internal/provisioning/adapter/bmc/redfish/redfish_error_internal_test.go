package redfish

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"syscall"
	"testing"

	"github.com/stmcginnis/gofish/schemas"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
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
			name:       "service temporarily unavailable",
			statusCode: 503,
			body:       `{"error":{"@Message.ExtendedInfo":[{"Message":"iDRAC is currently unable to display any information because data sources are unavailable.","MessageArgs":[],"MessageArgs@odata.count":0,"MessageId":"IDRAC.2.8.SYS518","RelatedProperties":[],"RelatedProperties@odata.count":0,"Resolution":"Wait for the data to be available and retry the operation. If the issue persists, contact your service provider.","Severity":"Informational"},{"Message":"The service is temporarily unavailable.  Retry in 30 seconds.","MessageArgs":["30"],"MessageArgs@odata.count":1,"MessageId":"Base.1.12.ServiceTemporarilyUnavailable","RelatedProperties":[],"RelatedProperties@odata.count":0,"Resolution":"Wait for the indicated retry duration and retry the operation.","Severity":"Critical"}],"code":"Base.1.12.GeneralError","message":"A general error has occurred. See ExtendedInfo for more information"}}`,

			want: "BMC returned HTTP 503: IDRAC.2.8.SYS518: iDRAC is currently unable to display any information because data sources are unavailable. (severity: Informational) Resolution: Wait for the data to be available and retry the operation. If the issue persists, contact your service provider.; Base.1.12.ServiceTemporarilyUnavailable: The service is temporarily unavailable.  Retry in 30 seconds. (severity: Critical) Resolution: Wait for the indicated retry duration and retry the operation.",
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

const serviceUnavailableBody = `{"error":{"@Message.ExtendedInfo":[{"Message":"iDRAC is currently unable to display any information because data sources are unavailable.","MessageArgs":[],"MessageArgs@odata.count":0,"MessageId":"IDRAC.2.8.SYS518","RelatedProperties":[],"RelatedProperties@odata.count":0,"Resolution":"Wait for the data to be available and retry the operation. If the issue persists, contact your service provider.","Severity":"Informational"},{"Message":"The service is temporarily unavailable.  Retry in 30 seconds.","MessageArgs":["30"],"MessageArgs@odata.count":1,"MessageId":"Base.1.12.ServiceTemporarilyUnavailable","RelatedProperties":[],"RelatedProperties@odata.count":0,"Resolution":"Wait for the indicated retry duration and retry the operation.","Severity":"Critical"}],"code":"Base.1.12.GeneralError","message":"A general error has occurred. See ExtendedInfo for more information"}}`

const serviceUnavailableMessage = "BMC returned HTTP 503: IDRAC.2.8.SYS518: iDRAC is currently unable to display any information because data sources are unavailable. (severity: Informational) Resolution: Wait for the data to be available and retry the operation. If the issue persists, contact your service provider.; Base.1.12.ServiceTemporarilyUnavailable: The service is temporarily unavailable.  Retry in 30 seconds. (severity: Critical) Resolution: Wait for the indicated retry duration and retry the operation."

func newCollectionError(failures map[string]error) error {
	collectionErr := schemas.NewCollectionError()
	maps.Copy(collectionErr.Failures, failures)

	return collectionErr
}

func TestWrapRedfishError_collection(t *testing.T) {
	err := wrapRedfishError(newCollectionError(map[string]error{
		"/redfish/v1/Systems/System.Embedded.1": schemas.ConstructError(http.StatusServiceUnavailable, []byte(serviceUnavailableBody)),
	}))

	require.EqualError(t, err, "/redfish/v1/Systems/System.Embedded.1: "+serviceUnavailableMessage, "The Redfish error response of the item the BMC could not serve is rendered instead of the raw response body gofish reports")

	var redfishErr *schemas.Error

	require.ErrorAs(t, err, &redfishErr, "The Redfish error response gofish collects in a map is reachable for the callers")
	require.Equal(t, http.StatusServiceUnavailable, redfishErr.HTTPReturnedStatusCode)

	require.Equal(t, err, wrapRedfishError(err), "Rendering the collected errors again leaves them unchanged")
}

func TestWrapRedfishError_collectionOrder(t *testing.T) {
	failures := map[string]error{
		"/redfish/v1/Systems/2": schemas.ConstructError(http.StatusNotFound, []byte(`{"error":{"code":"Base.1.0.ResourceMissingAtURI","message":"gone"}}`)),
		"/redfish/v1/Systems/1": schemas.ConstructError(http.StatusServiceUnavailable, []byte(`{"error":{"code":"Base.1.0.ServiceTemporarilyUnavailable","message":"busy"}}`)),
	}

	want := "/redfish/v1/Systems/1: BMC returned HTTP 503: Base.1.0.ServiceTemporarilyUnavailable: busy; /redfish/v1/Systems/2: BMC returned HTTP 404: Base.1.0.ResourceMissingAtURI: gone"

	for range 10 {
		require.EqualError(t, wrapRedfishError(newCollectionError(failures)), want, "The failures gofish collects in a map are rendered ordered by link, independent of the map iteration order")
	}
}

func TestWrapRedfishError_collectionWithoutRedfishError(t *testing.T) {
	err := wrapRedfishError(newCollectionError(map[string]error{
		"/redfish/v1/Systems/1": io.ErrUnexpectedEOF,
	}))

	require.EqualError(t, err, "/redfish/v1/Systems/1: unexpected EOF")
	require.ErrorIs(t, err, io.ErrUnexpectedEOF, "An item error which is not a Redfish error response stays reachable as well")
}

func TestRetryableWrapper(t *testing.T) {
	tests := []struct {
		name string
		err  error

		wantRetryable bool
	}{
		{
			name: "nil",
			err:  nil,
		},
		{
			name: "unrelated error",
			err:  errors.New("boom"),
		},
		{
			name:          "server error",
			err:           schemas.ConstructError(http.StatusInternalServerError, []byte(`{"error":{"code":"Base.1.0.InternalError","message":"boom"}}`)),
			wantRetryable: true,
		},
		{
			name:          "too many requests",
			err:           schemas.ConstructError(http.StatusTooManyRequests, []byte(`{"error":{"code":"Base.1.0.RateLimitExceeded","message":"slow down"}}`)),
			wantRetryable: true,
		},
		{
			name: "client error",
			err:  schemas.ConstructError(http.StatusBadRequest, []byte(`{"error":{"code":"Base.1.0.GeneralError","message":"boom"}}`)),
		},
		{
			name: "service unavailable for a collection item",
			err: fmt.Errorf("Failed to get BMC systems: %w", wrapRedfishError(newCollectionError(map[string]error{
				"/redfish/v1/Systems/System.Embedded.1": schemas.ConstructError(http.StatusServiceUnavailable, []byte(serviceUnavailableBody)),
			}))),
			wantRetryable: true,
		},
		{
			name: "service unavailable for a collection item, unrendered",
			err: fmt.Errorf("Failed to get BMC managers: %w", newCollectionError(map[string]error{
				"/redfish/v1/Managers/iDRAC.Embedded.1": schemas.ConstructError(http.StatusServiceUnavailable, []byte(serviceUnavailableBody)),
			})),
			wantRetryable: true,
		},
		{
			name: "client error for a collection item",
			err: fmt.Errorf("Failed to get BMC systems: %w", newCollectionError(map[string]error{
				"/redfish/v1/Systems/1": schemas.ConstructError(http.StatusNotFound, []byte(`{"error":{"code":"Base.1.0.ResourceMissingAtURI","message":"gone"}}`)),
			})),
		},
		{
			name: "one of the collection items is worth retrying",
			err: fmt.Errorf("Failed to get BMC systems: %w", newCollectionError(map[string]error{
				"/redfish/v1/Systems/1": schemas.ConstructError(http.StatusNotFound, []byte(`{"error":{"code":"Base.1.0.ResourceMissingAtURI","message":"gone"}}`)),
				"/redfish/v1/Systems/2": schemas.ConstructError(http.StatusServiceUnavailable, []byte(serviceUnavailableBody)),
			})),
			wantRetryable: true,
		},
		{
			name: "connection error for a collection item",
			err: fmt.Errorf("Failed to get BMC systems: %w", newCollectionError(map[string]error{
				"/redfish/v1/Systems/1": syscall.ECONNREFUSED,
			})),
			wantRetryable: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := RetryableWrapper()(tc.err)

			require.Equal(t, tc.wantRetryable, domain.IsRetryableError(err))

			switch {
			case tc.err == nil:
				require.NoError(t, err)

			case tc.wantRetryable:
				require.EqualError(t, errors.Unwrap(err), tc.err.Error(), "Marking the error as retryable leaves the error reported to the caller untouched")

			default:
				require.Equal(t, tc.err, err)
			}
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
