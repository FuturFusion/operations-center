package redfish

import (
	"encoding/json"
	"testing"

	"github.com/stmcginnis/gofish/schemas"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
)

func TestBootSourcesForMediaTypes(t *testing.T) {
	tests := []struct {
		name       string
		mediaTypes []schemas.VirtualMediaType

		want []schemas.BootSource
	}{
		{
			name:       "optical slot",
			mediaTypes: []schemas.VirtualMediaType{schemas.CDVirtualMediaType, schemas.DVDVirtualMediaType},

			want: []schemas.BootSource{schemas.CdBootSource},
		},
		{
			name:       "removable slot",
			mediaTypes: []schemas.VirtualMediaType{schemas.FloppyVirtualMediaType, schemas.USBStickVirtualMediaType},

			want: []schemas.BootSource{schemas.FloppyBootSource, schemas.UsbBootSource},
		},
		{
			name:       "media type nothing boots from",
			mediaTypes: []schemas.VirtualMediaType{"Tape"},

			want: []schemas.BootSource{},
		},
		{
			name:       "no media types reported",
			mediaTypes: nil,

			want: []schemas.BootSource{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, bootSourcesForMediaTypes(tc.mediaTypes))
		})
	}
}

func TestBootableMediaTypes(t *testing.T) {
	tests := []struct {
		name       string
		systemBody string
		mediaTypes []schemas.VirtualMediaType
		mediaURL   string

		want      []schemas.VirtualMediaType
		assertErr require.ErrorAssertionFunc
	}{
		{
			name:       "ISO in an optical slot",
			systemBody: `{"@odata.id":"/redfish/v1/Systems/1","Boot":{}}`,
			mediaTypes: []schemas.VirtualMediaType{schemas.CDVirtualMediaType, schemas.DVDVirtualMediaType},
			mediaURL:   "https://example.com/file.iso",

			want:      []schemas.VirtualMediaType{schemas.CDVirtualMediaType, schemas.DVDVirtualMediaType},
			assertErr: require.NoError,
		},
		{
			name:       "raw image in a slot taking floppy and USB",
			systemBody: `{"@odata.id":"/redfish/v1/Systems/1","Boot":{}}`,
			mediaTypes: []schemas.VirtualMediaType{schemas.FloppyVirtualMediaType, schemas.USBStickVirtualMediaType},
			mediaURL:   "https://example.com/file.raw",

			want:      []schemas.VirtualMediaType{schemas.USBStickVirtualMediaType, schemas.FloppyVirtualMediaType},
			assertErr: require.NoError,
		},
		{
			name:       "image with an unknown extension",
			systemBody: `{"@odata.id":"/redfish/v1/Systems/1","Boot":{}}`,
			mediaTypes: []schemas.VirtualMediaType{schemas.FloppyVirtualMediaType, schemas.USBStickVirtualMediaType},
			mediaURL:   "https://example.com/image",

			want:      []schemas.VirtualMediaType{schemas.FloppyVirtualMediaType, schemas.USBStickVirtualMediaType},
			assertErr: require.NoError,
		},
		{
			name:       "slot reporting no media types",
			systemBody: `{"@odata.id":"/redfish/v1/Systems/1","Boot":{}}`,
			mediaTypes: nil,
			mediaURL:   "https://example.com/file.iso",

			want:      []schemas.VirtualMediaType{schemas.CDVirtualMediaType, schemas.DVDVirtualMediaType},
			assertErr: require.NoError,
		},
		{
			name:       "allowable targets decide between the candidates",
			systemBody: `{"@odata.id":"/redfish/v1/Systems/1","Boot":{"BootSourceOverrideTarget@Redfish.AllowableValues":["None","Pxe","Usb"]}}`,
			mediaTypes: []schemas.VirtualMediaType{schemas.FloppyVirtualMediaType, schemas.USBStickVirtualMediaType},
			mediaURL:   "https://example.com/image",

			want:      []schemas.VirtualMediaType{schemas.USBStickVirtualMediaType},
			assertErr: require.NoError,
		},
		{
			name:       "allowable targets drop the preferred candidate",
			systemBody: `{"@odata.id":"/redfish/v1/Systems/1","Boot":{"BootSourceOverrideTarget@Redfish.AllowableValues":["None","Pxe","Floppy"]}}`,
			mediaTypes: []schemas.VirtualMediaType{schemas.CDVirtualMediaType, schemas.DVDVirtualMediaType, schemas.FloppyVirtualMediaType, schemas.USBStickVirtualMediaType},
			mediaURL:   "https://example.com/file.raw",

			want:      []schemas.VirtualMediaType{schemas.FloppyVirtualMediaType},
			assertErr: require.NoError,
		},
		{
			name:       "media type nothing boots from is left out",
			systemBody: `{"@odata.id":"/redfish/v1/Systems/1","Boot":{}}`,
			mediaTypes: []schemas.VirtualMediaType{"Tape", schemas.CDVirtualMediaType},
			mediaURL:   "https://example.com/image",

			want:      []schemas.VirtualMediaType{schemas.CDVirtualMediaType},
			assertErr: require.NoError,
		},
		{
			name:       "server does not boot from the slot",
			systemBody: `{"@odata.id":"/redfish/v1/Systems/1","Boot":{"BootSourceOverrideTarget@Redfish.AllowableValues":["None","Pxe","Hdd"]}}`,
			mediaTypes: []schemas.VirtualMediaType{schemas.CDVirtualMediaType},
			mediaURL:   "https://example.com/file.iso",

			assertErr: errassert.Contains(`Server cannot be set to boot from virtual media "system:1", it boots from: None, Pxe, Hdd`),
		},
		{
			name:       "nothing boots from the media type of the slot",
			systemBody: `{"@odata.id":"/redfish/v1/Systems/1","Boot":{}}`,
			mediaTypes: []schemas.VirtualMediaType{"Tape"},
			mediaURL:   "https://example.com/image",

			assertErr: errassert.Contains(`Virtual media "system:1" does not report a media type the server knows how to boot from`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			system := parseComputerSystem(t, tc.systemBody)
			virtualMedia := virtualMediaSlot{VirtualMedia: &schemas.VirtualMedia{MediaTypes: tc.mediaTypes}}

			got, err := bootableMediaTypes(system, mediaTypesForImage(virtualMedia, tc.mediaURL), "system:1")

			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestBootSourceOverrideEnabledCandidates(t *testing.T) {
	tests := []struct {
		name       string
		systemBody string

		want      []schemas.BootSourceOverrideEnabled
		assertErr require.ErrorAssertionFunc
	}{
		{
			name:       "BMC declaring nothing leaves every candidate to be tried",
			systemBody: `{"@odata.id":"/redfish/v1/Systems/1","Boot":{}}`,

			want:      []schemas.BootSourceOverrideEnabled{schemas.OnceBootSourceOverrideEnabled, schemas.ContinuousBootSourceOverrideEnabled},
			assertErr: require.NoError,
		},
		{
			name:       "BMC offering both",
			systemBody: `{"@odata.id":"/redfish/v1/Systems/1","Boot":{"BootSourceOverrideEnabled@Redfish.AllowableValues":["Disabled","Once","Continuous"]}}`,

			want:      []schemas.BootSourceOverrideEnabled{schemas.OnceBootSourceOverrideEnabled, schemas.ContinuousBootSourceOverrideEnabled},
			assertErr: require.NoError,
		},
		{
			name:       "BMC offering no one-time override",
			systemBody: `{"@odata.id":"/redfish/v1/Systems/1","Boot":{"BootSourceOverrideEnabled@Redfish.AllowableValues":["Disabled","Continuous"]}}`,

			want:      []schemas.BootSourceOverrideEnabled{schemas.ContinuousBootSourceOverrideEnabled},
			assertErr: require.NoError,
		},
		{
			name:       "BMC offering no override at all",
			systemBody: `{"@odata.id":"/redfish/v1/Systems/1","Boot":{"BootSourceOverrideEnabled@Redfish.AllowableValues":["Disabled"]}}`,

			assertErr: errassert.Contains("BMC offers neither a one-time nor a continuous boot device override, it offers: Disabled"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := bootSourceOverrideEnabledCandidates(parseComputerSystem(t, tc.systemBody))

			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestIsValueRejected(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		property string
		value    string

		want bool
	}{
		{
			name:     "value not in list naming the property in the message args",
			body:     `{"error":{"code":"Base.1.5.PropertyValueNotInList","@Message.ExtendedInfo":[{"MessageId":"Base.1.5.PropertyValueNotInList","MessageArgs":["Once","BootSourceOverrideEnabled"]}]}}`,
			property: "BootSourceOverrideEnabled",
			value:    "Once",

			want: true,
		},
		{
			name:     "value not supported naming the value in the message only",
			body:     `{"error":{"code":"Base.1.5.PropertyValueNotSupported","message":"The value None for the property BootSourceOverrideTarget is not supported."}}`,
			property: "BootSourceOverrideTarget",
			value:    "None",

			want: true,
		},
		{
			name:     "property not writable",
			body:     `{"error":{"code":"Base.1.0.PropertyNotWritable","@Message.ExtendedInfo":[{"MessageId":"Base.1.0.PropertyNotWritable","RelatedProperties":["#/BootSourceOverrideEnabled"]}]}}`,
			property: "BootSourceOverrideEnabled",
			value:    "Once",

			want: true,
		},
		{
			name:     "different property",
			body:     `{"error":{"code":"Base.1.5.PropertyValueNotInList","@Message.ExtendedInfo":[{"MessageId":"Base.1.5.PropertyValueNotInList","MessageArgs":["Cd","BootSourceOverrideTarget"]}]}}`,
			property: "BootSourceOverrideEnabled",
			value:    "Once",

			want: false,
		},
		{
			name:     "unrelated message id",
			body:     `{"error":{"code":"Base.1.5.InsufficientPrivilege","@Message.ExtendedInfo":[{"MessageId":"Base.1.5.InsufficientPrivilege","MessageArgs":["BootSourceOverrideEnabled"]}]}}`,
			property: "BootSourceOverrideEnabled",
			value:    "Once",

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := schemas.ConstructError(400, []byte(tc.body))

			require.Equal(t, tc.want, isValueRejected(err, tc.property, tc.value))
		})
	}
}

func TestRestoreDefaultBootDevice_leavesUnrelatedOverridesAlone(t *testing.T) {
	tests := []struct {
		name       string
		systemBody string
	}{
		{
			name:       "no override in place",
			systemBody: `{"@odata.id":"/redfish/v1/Systems/1","Boot":{"BootSourceOverrideEnabled":"Disabled","BootSourceOverrideTarget":"None"}}`,
		},
		{
			name:       "BMC reporting no override at all",
			systemBody: `{"@odata.id":"/redfish/v1/Systems/1","Boot":{}}`,
		},
		{
			name:       "override pointing somewhere else",
			systemBody: `{"@odata.id":"/redfish/v1/Systems/1","Boot":{"BootSourceOverrideEnabled":"Once","BootSourceOverrideTarget":"Pxe"}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			restored, err := restoreDefaultBootDevice(parseComputerSystem(t, tc.systemBody), nil, []schemas.BootSource{schemas.CdBootSource})

			require.NoError(t, err)
			require.False(t, restored)
		})
	}
}

func parseComputerSystem(t *testing.T, body string) *schemas.ComputerSystem {
	t.Helper()

	var system schemas.ComputerSystem

	require.NoError(t, json.Unmarshal([]byte(body), &system))

	return &system
}
