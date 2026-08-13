package redfish

import (
	"testing"

	"github.com/stmcginnis/gofish/schemas"
	"github.com/stretchr/testify/require"
)

func Test_registryFileURI(t *testing.T) {
	tests := []struct {
		name      string
		locations []schemas.MessageRegistryFileLocation

		want string
	}{
		{
			name: "english location is preferred over the others",
			locations: []schemas.MessageRegistryFileLocation{
				{Language: "de", URI: "/redfish/v1/registries/de/base.json"},
				{Language: "en", URI: "/redfish/v1/registries/en/base.json"},
			},

			want: "/redfish/v1/registries/en/base.json",
		},
		{
			name: "language tag with a region still counts as english",
			locations: []schemas.MessageRegistryFileLocation{
				{Language: "de", URI: "/redfish/v1/registries/de/base.json"},
				{Language: "en-US", URI: "/redfish/v1/registries/en-US/base.json"},
			},

			want: "/redfish/v1/registries/en-US/base.json",
		},
		{
			name: "language is matched case insensitively",
			locations: []schemas.MessageRegistryFileLocation{
				{Language: "de", URI: "/redfish/v1/registries/de/base.json"},
				{Language: "EN", URI: "/redfish/v1/registries/en/base.json"},
			},

			want: "/redfish/v1/registries/en/base.json",
		},
		{
			// BMCs reporting "default" instead of a language code, or no
			// language at all, still get their registry read.
			name: "first usable location is taken when none is english",
			locations: []schemas.MessageRegistryFileLocation{
				{Language: "default", URI: "/redfish/v1/registries/base.json"},
				{Language: "de", URI: "/redfish/v1/registries/de/base.json"},
			},

			want: "/redfish/v1/registries/base.json",
		},
		{
			name: "location without a uri is skipped",
			locations: []schemas.MessageRegistryFileLocation{
				{Language: "en"},
				{Language: "de", URI: "/redfish/v1/registries/de/base.json"},
			},

			want: "/redfish/v1/registries/de/base.json",
		},
		{
			// The canonical copy of the registry sits with its publisher out on
			// the internet. Reading it would have the daemon fetch from a third
			// party because a device said so.
			name: "publication uri is never used",
			locations: []schemas.MessageRegistryFileLocation{
				{Language: "en", PublicationURI: "https://redfish.dmtf.org/registries/Base.1.0.0.json"},
			},

			want: "",
		},
		{
			// Firmware is not held to the schema, which requires the URI to be
			// colocated with the service.
			name: "absolute uri is skipped, a rooted one is used instead",
			locations: []schemas.MessageRegistryFileLocation{
				{Language: "en", URI: "https://redfish.dmtf.org/registries/Base.1.0.0.json"},
				{Language: "de", URI: "/redfish/v1/registries/de/base.json"},
			},

			want: "/redfish/v1/registries/de/base.json",
		},
		{
			// Would be appended to the endpoint without a separator, ending up
			// at a host of its own.
			name: "relative uri without a leading slash is skipped",
			locations: []schemas.MessageRegistryFileLocation{
				{Language: "en", URI: "base.json"},
			},

			want: "",
		},
		{
			name: "archive uri is not followed",
			locations: []schemas.MessageRegistryFileLocation{
				{Language: "en", ArchiveURI: "/redfish/v1/registries/registries.zip", ArchiveFile: "base.json"},
			},

			want: "",
		},
		{
			name:      "no locations at all",
			locations: nil,

			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, registryFileURI(&schemas.MessageRegistryFile{Location: tc.locations}))
		})
	}
}

func Test_splitMessageID(t *testing.T) {
	tests := []struct {
		name      string
		messageID string

		wantPrefix string
		wantKey    string
		wantOK     bool
	}{
		{
			name:      "registry id as the specification defines it",
			messageID: "Base.1.0.PropertyUnknown",

			wantPrefix: "Base",
			wantKey:    "PropertyUnknown",
			wantOK:     true,
		},
		{
			// HPE iLO reports this for a message of the Base registry version
			// 1.0.0, which is why the version is not matched at all.
			name:      "version the registry itself does not carry",
			messageID: "Base.0.10.PropertyUnknown",

			wantPrefix: "Base",
			wantKey:    "PropertyUnknown",
			wantOK:     true,
		},
		{
			name:      "version with an errata level",
			messageID: "Base.1.18.1.PropertyMissing",

			wantPrefix: "Base",
			wantKey:    "PropertyMissing",
			wantOK:     true,
		},
		{
			name:      "vendor registry",
			messageID: "iLO.2.15.ResourceNotReadyRetry",

			wantPrefix: "iLO",
			wantKey:    "ResourceNotReadyRetry",
			wantOK:     true,
		},
		{
			name:      "no registry to look the message up in",
			messageID: "PropertyUnknown",
		},
		{
			name:      "no message key",
			messageID: "Base.1.0.",
		},
		{
			name:      "no registry prefix",
			messageID: ".PropertyUnknown",
		},
		{
			name:      "empty",
			messageID: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prefix, key, ok := splitMessageID(tc.messageID)

			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.wantPrefix, prefix)
			require.Equal(t, tc.wantKey, key)
		})
	}
}

func Test_expandMessageArgs(t *testing.T) {
	tests := []struct {
		name    string
		message string
		args    []string

		want string
	}{
		{
			name:    "single argument",
			message: "The property %1 is not in the list of valid properties for the resource.",
			args:    []string{"Inserted"},

			want: "The property Inserted is not in the list of valid properties for the resource.",
		},
		{
			name:    "several arguments",
			message: "The value %1 for the property %2 is not in the list of acceptable values.",
			args:    []string{"CIFS", "TransferProtocolType"},

			want: "The value CIFS for the property TransferProtocolType is not in the list of acceptable values.",
		},
		{
			name:    "argument used more than once",
			message: "The property %1 is read only. Remove %1 from the request body.",
			args:    []string{"Inserted"},

			want: "The property Inserted is read only. Remove Inserted from the request body.",
		},
		{
			// The BMC reported fewer arguments than the registry expects, which
			// leaves the placeholder in place rather than losing the rest of the
			// message.
			name:    "fewer arguments than placeholders",
			message: "The value %1 for the property %2 is of a different format than the property can accept.",
			args:    []string{"http://example.com/install.iso"},

			want: "The value http://example.com/install.iso for the property %2 is of a different format than the property can accept.",
		},
		{
			name:    "message without placeholders",
			message: "A general error has occurred.",
			args:    []string{"Inserted"},

			want: "A general error has occurred.",
		},
		{
			name:    "no arguments",
			message: "The property %1 is not in the list of valid properties for the resource.",

			want: "The property %1 is not in the list of valid properties for the resource.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, expandMessageArgs(tc.message, tc.args))
		})
	}
}
