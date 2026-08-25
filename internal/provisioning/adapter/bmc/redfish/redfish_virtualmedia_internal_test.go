package redfish

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stmcginnis/gofish/schemas"
	"github.com/stretchr/testify/require"
)

func TestVirtualMediaHasMedia(t *testing.T) {
	tests := []struct {
		name         string
		virtualMedia schemas.VirtualMedia

		want bool
	}{
		{
			name:         "free slot",
			virtualMedia: schemas.VirtualMedia{Inserted: new(false)},

			want: false,
		},
		{
			name:         "occupied slot",
			virtualMedia: schemas.VirtualMedia{Inserted: new(true), Image: "http://example.com/install.iso"},

			want: true,
		},
		{
			// AMI MegaRAC keeps the image URI of the last redirection around
			// after ejecting it.
			name:         "stale image without inserted",
			virtualMedia: schemas.VirtualMedia{Inserted: new(false), Image: "/mnt/tank/iso"},

			want: false,
		},
		{
			// HPE iLO reports a slot as inserted as soon as an image URI is
			// configured, even one it could not reach.
			name:         "inserted without image",
			virtualMedia: schemas.VirtualMedia{Inserted: new(true)},

			want: false,
		},
		{
			name:         "image only, inserted not reported",
			virtualMedia: schemas.VirtualMedia{Image: "http://example.com/install.iso"},

			want: true,
		},
		{
			name:         "image name only, inserted not reported",
			virtualMedia: schemas.VirtualMedia{ImageName: "install.iso"},

			want: true,
		},
		{
			name:         "nothing reported",
			virtualMedia: schemas.VirtualMedia{},

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, virtualMediaHasMedia(virtualMediaSlot{VirtualMedia: &tc.virtualMedia}))
		})
	}
}

func TestTransferProtocolTypeForURL(t *testing.T) {
	tests := []struct {
		mediaURL string

		want schemas.TransferProtocolType
	}{
		{mediaURL: "https://example.com/install.iso", want: schemas.HTTPSTransferProtocolType},
		{mediaURL: "http://example.com/install.iso", want: schemas.HTTPTransferProtocolType},
		{mediaURL: "HTTPS://example.com/install.iso", want: schemas.HTTPSTransferProtocolType},
		{mediaURL: "https://[2602:fc62:b:3050::1]:8443/1.0/provisioning/file.iso", want: schemas.HTTPSTransferProtocolType},
		{mediaURL: "/mnt/tank/iso", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.mediaURL, func(t *testing.T) {
			require.Equal(t, tc.want, transferProtocolTypeForURL(tc.mediaURL))
		})
	}
}

func TestMediaTypesForURL(t *testing.T) {
	tests := []struct {
		mediaURL string

		want []schemas.VirtualMediaType
	}{
		{
			mediaURL: "https://example.com/1.0/provisioning/tokens/x/seeds/y/type/iso/file.iso",
			want:     []schemas.VirtualMediaType{schemas.CDVirtualMediaType, schemas.DVDVirtualMediaType},
		},
		{
			mediaURL: "https://example.com/file.ISO",
			want:     []schemas.VirtualMediaType{schemas.CDVirtualMediaType, schemas.DVDVirtualMediaType},
		},
		{
			mediaURL: "https://example.com/file.img",
			want:     []schemas.VirtualMediaType{schemas.USBStickVirtualMediaType, schemas.FloppyVirtualMediaType},
		},
		{
			mediaURL: "https://example.com/file.iso?token=abc",
			want:     []schemas.VirtualMediaType{schemas.CDVirtualMediaType, schemas.DVDVirtualMediaType},
		},
		{
			mediaURL: "https://example.com/image",
			want:     nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.mediaURL, func(t *testing.T) {
			require.Equal(t, tc.want, mediaTypesForURL(tc.mediaURL))
		})
	}
}

func TestMediaTypeForInsert(t *testing.T) {
	tests := []struct {
		name            string
		slotMediaTypes  []schemas.VirtualMediaType
		mediaURL        string
		allowableValues []string

		want schemas.VirtualMediaType
	}{
		{
			name:           "raw image on a slot taking every media type",
			slotMediaTypes: []schemas.VirtualMediaType{schemas.CDVirtualMediaType, schemas.DVDVirtualMediaType, schemas.FloppyVirtualMediaType, schemas.USBStickVirtualMediaType},
			mediaURL:       "https://example.com/file.raw",

			want: schemas.USBStickVirtualMediaType,
		},
		{
			name:           "ISO image on a slot taking every media type",
			slotMediaTypes: []schemas.VirtualMediaType{schemas.CDVirtualMediaType, schemas.DVDVirtualMediaType, schemas.FloppyVirtualMediaType, schemas.USBStickVirtualMediaType},
			mediaURL:       "https://example.com/file.iso",

			want: schemas.CDVirtualMediaType,
		},
		{
			name:           "slot taking a single media type",
			slotMediaTypes: []schemas.VirtualMediaType{schemas.DVDVirtualMediaType},
			mediaURL:       "https://example.com/file.iso",

			want: schemas.DVDVirtualMediaType,
		},
		{
			name:     "slot reporting no media types",
			mediaURL: "https://example.com/file.raw",

			want: schemas.USBStickVirtualMediaType,
		},
		{
			name:           "image of an unrecognized kind",
			slotMediaTypes: []schemas.VirtualMediaType{schemas.CDVirtualMediaType, schemas.DVDVirtualMediaType},
			mediaURL:       "https://example.com/image",

			want: "",
		},
		{
			name:           "slot taking none of the media types the image fits",
			slotMediaTypes: []schemas.VirtualMediaType{schemas.FloppyVirtualMediaType, schemas.USBStickVirtualMediaType},
			mediaURL:       "https://example.com/file.iso",

			want: "",
		},
		{
			name:            "allowable values narrow the choice down",
			slotMediaTypes:  []schemas.VirtualMediaType{schemas.CDVirtualMediaType, schemas.DVDVirtualMediaType, schemas.FloppyVirtualMediaType, schemas.USBStickVirtualMediaType},
			mediaURL:        "https://example.com/file.raw",
			allowableValues: []string{"CD", "DVD", "Floppy"},

			want: schemas.FloppyVirtualMediaType,
		},
		{
			name:            "allowable values leaving nothing the image fits",
			slotMediaTypes:  []schemas.VirtualMediaType{schemas.CDVirtualMediaType, schemas.DVDVirtualMediaType, schemas.FloppyVirtualMediaType, schemas.USBStickVirtualMediaType},
			mediaURL:        "https://example.com/file.raw",
			allowableValues: []string{"CD", "DVD"},

			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			virtualMedia := virtualMediaSlot{VirtualMedia: &schemas.VirtualMedia{MediaTypes: tc.slotMediaTypes}}

			require.Equal(t, tc.want, mediaTypeForInsert(tc.mediaURL, mediaTypesForImage(virtualMedia, tc.mediaURL), tc.allowableValues))
		})
	}
}

func TestIsParameterMissing(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		parameter string

		want bool
	}{
		{
			// AMI MegaRAC, pointing at the parameter with a JSON pointer without the leading "#".
			name:      "action parameter missing with related property",
			body:      `{"error":{"code":"Base.1.5.ActionParameterMissing","message":"The action requires the parameter TransferProtocolType.","@Message.ExtendedInfo":[{"MessageId":"Base.1.5.ActionParameterMissing","RelatedProperties":["/TransferProtocolType"]}]}}`,
			parameter: "TransferProtocolType",

			want: true,
		},
		{
			name:      "property missing with hash prefixed related property",
			body:      `{"error":{"code":"Base.1.0.PropertyMissing","@Message.ExtendedInfo":[{"MessageId":"Base.1.0.PropertyMissing","RelatedProperties":["#/TransferProtocolType"]}]}}`,
			parameter: "TransferProtocolType",

			want: true,
		},
		{
			// BMCs reporting no related properties name the parameter in the message arguments instead.
			name:      "property missing with message args only",
			body:      `{"error":{"code":"Base.1.18.1.PropertyMissing","@Message.ExtendedInfo":[{"MessageId":"Base.1.18.1.PropertyMissing","MessageArgs":["TransferProtocolType"]}]}}`,
			parameter: "TransferProtocolType",

			want: true,
		},
		{
			// Some report a generic error and name the parameter in the message text only.
			name:      "general error naming the parameter in the message",
			body:      `{"error":{"code":"Base.1.0.GeneralError","message":"The action VirtualMedia.InsertMedia requires the parameter TransferMethod to be present in the request body."}}`,
			parameter: "TransferMethod",

			want: true,
		},
		{
			name:      "different parameter",
			body:      `{"error":{"code":"Base.1.5.ActionParameterMissing","@Message.ExtendedInfo":[{"MessageId":"Base.1.5.ActionParameterMissing","RelatedProperties":["/TransferProtocolType"]}]}}`,
			parameter: "TransferMethod",

			want: false,
		},
		{
			name:      "unrelated message id",
			body:      `{"error":{"code":"Base.1.5.PropertyValueFormatError","@Message.ExtendedInfo":[{"MessageId":"Base.1.5.PropertyValueFormatError","RelatedProperties":["#/Image"]}]}}`,
			parameter: "Image",

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := schemas.ConstructError(400, []byte(tc.body))

			require.Equal(t, tc.want, isParameterMissing(err, tc.parameter))
		})
	}
}

func TestIsHintRejected(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		parameter string

		want bool
	}{
		{
			name:      "action parameter unknown",
			body:      `{"error":{"code":"Base.1.5.ActionParameterUnknown","message":"The action VirtualMedia.InsertMedia was submitted with the invalid parameter MediaType.","@Message.ExtendedInfo":[{"MessageId":"Base.1.5.ActionParameterUnknown","MessageArgs":["VirtualMedia.InsertMedia","MediaType"],"RelatedProperties":["/MediaType"]}]}}`,
			parameter: "MediaType",

			want: true,
		},
		{
			name:      "action parameter not supported",
			body:      `{"error":{"code":"Base.1.18.1.ActionParameterNotSupported","@Message.ExtendedInfo":[{"MessageId":"Base.1.18.1.ActionParameterNotSupported","MessageArgs":["MediaType","VirtualMedia.InsertMedia"]}]}}`,
			parameter: "MediaType",

			want: true,
		},
		{
			// BMCs treating the action parameters as resource properties.
			name:      "property unknown",
			body:      `{"error":{"code":"iLO.0.10.ExtendedInfo","@Message.ExtendedInfo":[{"MessageID":"Base.0.10.PropertyUnknown","MessageArgs":["MediaType"]}]}}`,
			parameter: "MediaType",

			want: true,
		},
		{
			name:      "value not in the list the BMC accepts",
			body:      `{"error":{"code":"Base.1.5.ActionParameterValueNotInList","@Message.ExtendedInfo":[{"MessageId":"Base.1.5.ActionParameterValueNotInList","MessageArgs":["USBStick","MediaType","VirtualMedia.InsertMedia"]}]}}`,
			parameter: "MediaType",

			want: true,
		},
		{
			name:      "general error naming the parameter in the message",
			body:      `{"error":{"code":"Base.1.0.GeneralError","message":"The parameter MediaType is not supported by this implementation."}}`,
			parameter: "MediaType",

			want: true,
		},
		{
			name:      "property value not in the list the BMC accepts",
			body:      `{"error":{"code":"Base.1.5.PropertyValueNotInList","@Message.ExtendedInfo":[{"MessageId":"Base.1.5.PropertyValueNotInList","MessageArgs":["USBStick","MediaType"]}]}}`,
			parameter: "MediaType",

			want: true,
		},
		{
			name:      "property value not supported",
			body:      `{"error":{"code":"Base.1.5.PropertyValueNotSupported","@Message.ExtendedInfo":[{"MessageId":"Base.1.5.PropertyValueNotSupported","RelatedProperties":["#/MediaType"]}]}}`,
			parameter: "MediaType",

			want: true,
		},
		{
			name:      "property value error",
			body:      `{"error":{"code":"Base.1.5.PropertyValueError","message":"The value for the property MediaType is invalid."}}`,
			parameter: "MediaType",

			want: true,
		},
		{
			name:      "property not writable",
			body:      `{"error":{"code":"Base.1.5.PropertyNotWritable","@Message.ExtendedInfo":[{"MessageId":"Base.1.5.PropertyNotWritable","MessageArgs":["MediaType"]}]}}`,
			parameter: "MediaType",

			want: true,
		},
		{
			// "MediaType" is a substring of the standard property "MediaTypes".
			name:      "general error naming a longer property the name is part of",
			body:      `{"error":{"code":"Base.1.0.GeneralError","message":"The image does not match the MediaTypes supported by this slot."}}`,
			parameter: "MediaType",

			want: false,
		},
		{
			name:      "different parameter",
			body:      `{"error":{"code":"Base.1.5.ActionParameterUnknown","@Message.ExtendedInfo":[{"MessageId":"Base.1.5.ActionParameterUnknown","RelatedProperties":["/TransferMethod"]}]}}`,
			parameter: "MediaType",

			want: false,
		},
		{
			name:      "parameter reported as missing, not rejected",
			body:      `{"error":{"code":"Base.1.5.ActionParameterMissing","@Message.ExtendedInfo":[{"MessageId":"Base.1.5.ActionParameterMissing","RelatedProperties":["/MediaType"]}]}}`,
			parameter: "MediaType",

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := schemas.ConstructError(400, []byte(tc.body))

			require.Equal(t, tc.want, isHintRejected(err, tc.parameter))
		})
	}
}

func TestIsPropertyRejected(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		property string

		want bool
	}{
		{
			name:     "property unknown",
			body:     `{"error":{"code":"iLO.0.10.ExtendedInfo","message":"See @Message.ExtendedInfo for more information.","@Message.ExtendedInfo":[{"MessageID":"Base.0.10.PropertyUnknown","MessageArgs":["Inserted"]}]}}`,
			property: "Inserted",

			want: true,
		},
		{
			name:     "property not writable",
			body:     `{"error":{"code":"Base.1.0.PropertyNotWritable","@Message.ExtendedInfo":[{"MessageId":"Base.1.0.PropertyNotWritable","RelatedProperties":["#/Inserted"]}]}}`,
			property: "Inserted",

			want: true,
		},
		{
			name:     "different property",
			body:     `{"error":{"code":"iLO.0.10.ExtendedInfo","@Message.ExtendedInfo":[{"MessageID":"Base.0.10.PropertyUnknown","MessageArgs":["TransferMethod"]}]}}`,
			property: "Inserted",

			want: false,
		},
		{
			name:     "property value format error",
			body:     `{"error":{"code":"Base.1.5.PropertyValueFormatError","@Message.ExtendedInfo":[{"MessageId":"Base.1.5.PropertyValueFormatError","RelatedProperties":["#/Image"]}]}}`,
			property: "Inserted",

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := schemas.ConstructError(400, []byte(tc.body))

			require.Equal(t, tc.want, isPropertyRejected(err, tc.property))
		})
	}
}

func TestIsParameterMissing_nonRedfishError(t *testing.T) {
	err := errors.New("connection refused")

	require.False(t, isParameterMissing(err, "TransferProtocolType"))
	require.False(t, isHintRejected(err, "MediaType"))
	require.False(t, isPropertyRejected(err, "Inserted"))
	require.False(t, isPreconditionRejected(err))
	require.False(t, isRequestRejected(err))
}

func TestIsPreconditionRejected(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string

		want bool
	}{
		{
			name:       "precondition failed",
			statusCode: http.StatusPreconditionFailed,
			body:       `{"error":{"code":"Base.1.5.PreconditionFailed","message":"The ETag supplied did not match."}}`,

			want: true,
		},
		{
			name:       "precondition required",
			statusCode: http.StatusPreconditionRequired,
			body:       `{"error":{"code":"Base.1.5.PreconditionRequired","message":"A precondition header is required."}}`,

			want: true,
		},
		{
			name:       "header rejected",
			statusCode: http.StatusBadRequest,
			body:       `{"error":{"code":"Base.1.0.GeneralError","@Message.ExtendedInfo":[{"MessageId":"Base.1.0.HeaderInvalid","MessageArgs":["If-Match"]}]}}`,

			want: true,
		},
		{
			name:       "precondition reported without a matching status code",
			statusCode: http.StatusBadRequest,
			body:       `{"error":{"code":"Base.1.5.PreconditionFailed"}}`,

			want: true,
		},
		{
			name:       "rejected property",
			statusCode: http.StatusBadRequest,
			body:       `{"error":{"code":"iLO.0.10.ExtendedInfo","@Message.ExtendedInfo":[{"MessageID":"Base.0.10.PropertyUnknown","MessageArgs":["Inserted"]}]}}`,

			want: false,
		},
		{
			name:       "generic client error",
			statusCode: http.StatusBadRequest,
			body:       `{"error":{"code":"Base.1.0.GeneralError","message":"Something went wrong."}}`,

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := schemas.ConstructError(tc.statusCode, []byte(tc.body))

			require.Equal(t, tc.want, isPreconditionRejected(err))
		})
	}
}

func TestIsRequestRejected(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string

		want bool
	}{
		{
			name:       "rejected parameter",
			statusCode: http.StatusBadRequest,
			body:       `{"error":{"code":"Base.1.5.ActionParameterUnknown","@Message.ExtendedInfo":[{"MessageId":"Base.1.5.ActionParameterUnknown","MessageArgs":["VirtualMedia.InsertMedia","MediaType"]}]}}`,

			want: true,
		},
		{
			// The case the media type hint is given up for: the BMC turned the
			// request down without saying anything which could be read.
			name:       "client error in no readable form",
			statusCode: http.StatusBadRequest,
			body:       `<html><body>Bad Request</body></html>`,

			want: true,
		},
		{
			name:       "not found",
			statusCode: http.StatusNotFound,
			body:       `{"error":{"code":"Base.1.5.ResourceMissingAtURI"}}`,

			want: true,
		},
		{
			name:       "conflict",
			statusCode: http.StatusConflict,
			body:       `{"error":{"code":"Base.1.5.ResourceInUse"}}`,

			want: true,
		},
		{
			// The BMC may well have attached the media before failing to report
			// it, so the request must not be repeated.
			name:       "internal server error",
			statusCode: http.StatusInternalServerError,
			body:       `{"error":{"code":"Base.1.0.InternalError"}}`,

			want: false,
		},
		{
			name:       "service unavailable",
			statusCode: http.StatusServiceUnavailable,
			body:       `{"error":{"code":"Base.1.5.ServiceTemporarilyUnavailable"}}`,

			want: false,
		},
		{
			name:       "unreadable response",
			statusCode: 0,
			body:       `unexpected EOF`,

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := schemas.ConstructError(tc.statusCode, []byte(tc.body))

			require.Equal(t, tc.want, isRequestRejected(err))
		})
	}
}

func TestFindOEMActionTarget(t *testing.T) {
	tests := []struct {
		name   string
		oem    string
		action string

		want string
	}{
		{
			// HPE iLO 4.
			name:   "action directly below oem",
			oem:    `{"#HpiLOVirtualMedia.InsertVirtualMedia":{"target":"/insert"}}`,
			action: "InsertVirtualMedia",

			want: "/insert",
		},
		{
			// HPE iLO 5.
			name:   "action grouped by vendor",
			oem:    `{"Hpe":{"#HpeiLOVirtualMedia.InsertVirtualMedia":{"target":"/insert"},"#HpeiLOVirtualMedia.EjectVirtualMedia":{"target":"/eject"}}}`,
			action: "EjectVirtualMedia",

			want: "/eject",
		},
		{
			name:   "action of a different name",
			oem:    `{"Hpe":{"#HpeiLOVirtualMedia.EjectVirtualMedia":{"target":"/eject"}}}`,
			action: "InsertVirtualMedia",

			want: "",
		},
		{
			name:   "action without a target",
			oem:    `{"Hpe":{"#HpeiLOVirtualMedia.InsertVirtualMedia":{}}}`,
			action: "InsertVirtualMedia",

			want: "",
		},
		{
			name:   "nested deeper than the search descends",
			oem:    `{"a":{"b":{"c":{"#Vendor.InsertVirtualMedia":{"target":"/insert"}}}}}`,
			action: "InsertVirtualMedia",

			want: "",
		},
		{
			name:   "no vendor specific actions at all",
			oem:    `{}`,
			action: "InsertVirtualMedia",

			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var oem map[string]any

			require.NoError(t, json.Unmarshal([]byte(tc.oem), &oem))
			require.Equal(t, tc.want, findOEMActionTarget(oem, tc.action, maxOEMActionDepth))
		})
	}
}

func TestOEMActionTarget(t *testing.T) {
	virtualMedia := virtualMediaSlot{VirtualMedia: &schemas.VirtualMedia{
		RawData: []byte(`{
  "@odata.id": "/redfish/v1/Managers/1/VirtualMedia/2",
  "Actions": {
    "Oem": {
      "Hpe": {
        "#HpeiLOVirtualMedia.InsertVirtualMedia": { "target": "/insert" }
      }
    }
  }
}`),
	}}

	require.Equal(t, "/insert", oemActionTarget(virtualMedia, "InsertVirtualMedia"))
	require.Empty(t, oemActionTarget(virtualMedia, "EjectVirtualMedia"))

	require.Empty(t, oemActionTarget(virtualMediaSlot{VirtualMedia: &schemas.VirtualMedia{}}, "InsertVirtualMedia"))
}
