package provisioning_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

const (
	testRewriteServerUUID = "4c4c4544-0042-3510-8051-b3c04f4e3332"
	testRewriteSerial     = "CN7792171D0074"
)

func testRewriteServer() provisioning.Server {
	return provisioning.Server{
		Name:          "r640-03",
		ConnectionURL: "https://r640-03.lab.example.net:8443",
		BMCConfig: api.BMCConfig{
			Endpoint: "https://192.168.1.100:8443",
			Username: "root",
			Password: "calvin",
		},
		BMCData: api.BMCData{
			ServerSerialNumber:       testRewriteSerial,
			ServerUUID:               testRewriteServerUUID,
			ServerHostName:           "r640-03.lab.example.net",
			ServerAssetTag:           "ASSET-42",
			BMCServiceIdentification: "7QZ1234",
			ServerBIOSAttributes: map[string]any{
				"NicMac1": "AA:BB:CC:DD:EE:01",
				"NicMac2": "aa-bb-cc-dd-ee-02",
			},
		},
	}
}

func testRewriteManifest() api.BMCRecordingManifest {
	return api.BMCRecordingManifest{
		Server:      "r640-03",
		BMCEndpoint: "https://192.168.1.100:8443",
		MediaURLs: []string{
			"https://10.0.0.5:7443/media/seed.iso",
			"https://10.0.0.5:7443/secure-boot-media/AAAA",
		},
	}
}

func TestNewBMCRecordingRewrite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		server   provisioning.Server
		manifest api.BMCRecordingManifest

		want *provisioning.BMCRecordingRewrite
	}{
		{
			name:     "every field set",
			server:   testRewriteServer(),
			manifest: testRewriteManifest(),

			want: &provisioning.BMCRecordingRewrite{
				GenericPatterns: true,
				Replacements: []provisioning.BMCRecordingReplacement{
					{Old: testRewriteServerUUID, New: "00000000-0000-4000-8000-000000000001", CaseInsensitive: true},
					{Old: "r640-03.lab.example.net", New: "server.example", WholeWord: true},
					{Old: "AA:BB:CC:DD:EE:01", New: "02:00:00:00:00:01", CaseInsensitive: true},
					{Old: "aa-bb-cc-dd-ee-02", New: "02:00:00:00:00:02", CaseInsensitive: true},
					{Old: testRewriteSerial, New: "SERIAL000001"},
					{Old: "192.168.1.100", New: "bmc.example", WholeWord: true},
					{Old: "10.0.0.5", New: "operations-center.example", WholeWord: true},
					{Old: "ASSET-42", New: "ASSET000001", WholeWord: true},
					{Old: "r640-03", New: "server01", WholeWord: true},
					{Old: "7QZ1234", New: "SERVICE000001", WholeWord: true},
					{Old: "calvin", New: "<redacted>", WholeWord: true},
					{Old: "root", New: "bmc-user", WholeWord: true},
				},
			},
		},
		{
			name:   "nothing known",
			server: provisioning.Server{},

			want: &provisioning.BMCRecordingRewrite{
				GenericPatterns: true,
				Replacements:    []provisioning.BMCRecordingReplacement{},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Run test
			got := provisioning.NewBMCRecordingRewrite(tc.server, tc.manifest)

			// Assert
			require.Equal(t, tc.want, got, "every known value is replaced once, the longest values first")
		})
	}
}

func TestNewBMCRecordingRewrite_Apply(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		line string

		want string
	}{
		{
			name: "exchange with URL and body",
			line: `{"url":"https://192.168.1.100:8443/redfish/v1/Systems/System.Embedded.1","response_body":"{\"SerialNumber\":\"CN7792171D0074\",\"UUID\":\"4C4C4544-0042-3510-8051-B3C04F4E3332\",\"Id\":\"4c4c4544-0042-3510-8051-b3c04f4e3332\"}"}`,

			want: `{"url":"https://bmc.example:8443/redfish/v1/Systems/System.Embedded.1","response_body":"{\"SerialNumber\":\"SERIAL000001\",\"UUID\":\"00000000-0000-4000-8000-000000000001\",\"Id\":\"00000000-0000-4000-8000-000000000001\"}"}`,
		},
		{
			name: "manifest",
			line: `  "server": "r640-03", "bmc_endpoint": "https://192.168.1.100:8443", "media_urls": ["https://10.0.0.5:7443/media/seed.iso"]`,

			want: `  "server": "server01", "bmc_endpoint": "https://bmc.example:8443", "media_urls": ["https://operations-center.example:7443/media/seed.iso"]`,
		},
		{
			name: "MAC address in another spelling",
			line: `"MACAddress":"AA-BB-CC-DD-EE-01","PermanentMACAddress":"aa:bb:cc:dd:ee:02"`,

			want: `"MACAddress":"02:00:00:00:00:01","PermanentMACAddress":"02:00:00:00:00:02"`,
		},
		{
			name: "short values only as whole words",
			line: `"UserName":"root","RootService":"/redfish/v1","Name":"r640-03b","Host":"r640-03"`,

			want: `"UserName":"bmc-user","RootService":"/redfish/v1","Name":"r640-03b","Host":"server01"`,
		},
		{
			name: "no match",
			line: `{"method":"GET","url":"/redfish/v1/Managers"}`,

			want: `{"method":"GET","url":"/redfish/v1/Managers"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			rewrite := provisioning.NewBMCRecordingRewrite(testRewriteServer(), testRewriteManifest())

			// Run test
			got := rewrite.Apply(tc.line)

			// Assert
			require.Equal(t, tc.want, got)
		})
	}
}

func TestBMCRecordingRewrite_ApplyGenericPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		replacements []provisioning.BMCRecordingReplacement
		lines        []string

		want []string
	}{
		{
			name: "MAC addresses are mapped stably",
			lines: []string{
				`"a":"AA:BB:CC:00:00:01","b":"aa:bb:cc:00:00:02"`,
				`"c":"aa-bb-cc-00-00-01"`,
			},

			want: []string{
				`"a":"02:00:00:00:00:01","b":"02:00:00:00:00:02"`,
				`"c":"02:00:00:00:00:01"`,
			},
		},
		{
			name: "IPv4 address in URL and body",
			lines: []string{
				`{"url":"https://10.1.2.3/redfish/v1","response_body":"{\"Address\":\"10.1.2.3\",\"Gateway\":\"10.1.2.1\"}"}`,
			},

			want: []string{
				`{"url":"https://203.0.113.1/redfish/v1","response_body":"{\"Address\":\"203.0.113.1\",\"Gateway\":\"203.0.113.2\"}"}`,
			},
		},
		{
			name: "IPv4 addresses, that identify nothing, stay",
			lines: []string{
				`"SubnetMask":"255.255.255.0","Loopback":"127.0.0.1","Any":"0.0.0.0","Version":"999.1.2.3"`,
			},

			want: []string{
				`"SubnetMask":"255.255.255.0","Loopback":"127.0.0.1","Any":"0.0.0.0","Version":"999.1.2.3"`,
			},
		},
		{
			name: "UUIDs are mapped stably regardless of case",
			lines: []string{
				`"Task":"/redfish/v1/TaskService/Tasks/0B7A8C9D-1E2F-4A5B-8C6D-7E8F9A0B1C2D"`,
				`"Id":"0b7a8c9d-1e2f-4a5b-8c6d-7e8f9a0b1c2d"`,
			},

			want: []string{
				`"Task":"/redfish/v1/TaskService/Tasks/00000000-0000-4000-8000-000000000001"`,
				`"Id":"00000000-0000-4000-8000-000000000001"`,
			},
		},
		{
			name: "placeholders of explicit replacements stay and are not handed out again",
			replacements: []provisioning.BMCRecordingReplacement{
				{Old: "AA:BB:CC:00:00:01", New: "02:00:00:00:00:01", CaseInsensitive: true},
				{Old: "192.168.1.100", New: "bmc.example"},
			},
			lines: []string{
				`"b":"aa:bb:cc:00:00:02","c":"02:00:00:00:00:01"`,
				`"a":"aa:bb:cc:00:00:01","host":"192.168.1.100"`,
			},

			want: []string{
				`"b":"02:00:00:00:00:02","c":"02:00:00:00:00:01"`,
				`"a":"02:00:00:00:00:01","host":"bmc.example"`,
			},
		},
		{
			name: "explicit value in another spelling gets the explicit placeholder",
			replacements: []provisioning.BMCRecordingReplacement{
				{Old: "AA:BB:CC:00:00:01", New: "02:00:00:00:00:07"},
			},
			lines: []string{
				`"b":"aa:bb:cc:00:00:02","a":"aa-bb-cc-00-00-01"`,
			},

			want: []string{
				`"b":"02:00:00:00:00:01","a":"02:00:00:00:00:07"`,
			},
		},
		{
			name: "line without match",
			lines: []string{
				`{"method":"GET","url":"/redfish/v1/Systems/1"}`,
			},

			want: []string{
				`{"method":"GET","url":"/redfish/v1/Systems/1"}`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			rewrite := &provisioning.BMCRecordingRewrite{
				Replacements:    tc.replacements,
				GenericPatterns: true,
			}

			// Run test
			got := make([]string, 0, len(tc.lines))
			for _, line := range tc.lines {
				got = append(got, rewrite.Apply(line))
			}

			// Assert
			require.Equal(t, tc.want, got, "the same value gets the same placeholder on every line")
		})
	}
}
