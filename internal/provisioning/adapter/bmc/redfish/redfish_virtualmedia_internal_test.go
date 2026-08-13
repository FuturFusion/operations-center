package redfish

import (
	"errors"
	"testing"

	"github.com/stmcginnis/gofish/schemas"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/ptr"
)

func TestVirtualMediaHasMedia(t *testing.T) {
	tests := []struct {
		name         string
		virtualMedia schemas.VirtualMedia

		want bool
	}{
		{
			name:         "free slot",
			virtualMedia: schemas.VirtualMedia{Inserted: ptr.To(false)},

			want: false,
		},
		{
			name:         "occupied slot",
			virtualMedia: schemas.VirtualMedia{Inserted: ptr.To(true), Image: "http://example.com/install.iso"},

			want: true,
		},
		{
			// AMI MegaRAC keeps the image URI of the last redirection around
			// after ejecting it.
			name:         "stale image without inserted",
			virtualMedia: schemas.VirtualMedia{Inserted: ptr.To(false), Image: "/mnt/tank/iso"},

			want: false,
		},
		{
			// HPE iLO reports a slot as inserted as soon as an image URI is
			// configured, even one it could not reach.
			name:         "inserted without image",
			virtualMedia: schemas.VirtualMedia{Inserted: ptr.To(true)},

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
			require.Equal(t, tc.want, virtualMediaHasMedia(&tc.virtualMedia))
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
	require.False(t, isPropertyRejected(err, "Inserted"))
}
