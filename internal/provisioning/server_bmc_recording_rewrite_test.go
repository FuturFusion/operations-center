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
	testRewriteTokenUUID  = "8f6c3d1a-2b4e-4c9a-9f7d-1a2b3c4d5e6f"
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
			ServerSKU:                "SKU-R640-X",
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
		Request: api.ServerDeploymentPost{
			TokenUUID: testRewriteTokenUUID,
		},
		MediaURLs: []string{
			"https://10.0.0.5:7443/media/seed.iso",
			"https://10.0.0.5:7443/secure-boot-media/AAAA",
		},
	}
}

func TestNewBMCRecordingRewrite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		server                  provisioning.Server
		manifest                api.BMCRecordingManifest
		operationsCenterAddress string

		want *provisioning.BMCRecordingRewrite
	}{
		{
			name:                    "every field set",
			server:                  testRewriteServer(),
			manifest:                testRewriteManifest(),
			operationsCenterAddress: "https://oc.corp.example.net:7443",

			want: &provisioning.BMCRecordingRewrite{
				GenericPatterns: true,
				Replacements: []provisioning.BMCRecordingReplacement{
					{Old: "192.168.1.100", New: "bmc.example", CaseInsensitive: true, WholeWord: true},
					{Old: "root", New: "bmc-user", WholeWord: true},
					{Old: "calvin", New: "<redacted>", WholeWord: true},
					{Old: "r640-03.lab.example.net", New: "server.example", CaseInsensitive: true, WholeWord: true},
					{Old: "r640-03", New: "server01", WholeWord: true},
					{Old: "oc.corp.example.net", New: "operations-center.example", CaseInsensitive: true, WholeWord: true},
					{Old: "10.0.0.5", New: "operations-center.example", CaseInsensitive: true, WholeWord: true},
					{Old: testRewriteTokenUUID, New: "00000000-0000-4000-8000-000000000002", CaseInsensitive: true},
					{Old: testRewriteSerial, New: "SERIAL000001"},
					{Old: "ASSET-42", New: "ASSET000001", WholeWord: true},
					{Old: "7QZ1234", New: "SERVICE000001", WholeWord: true},
					{Old: "SKU-R640-X", New: "SKU000001", WholeWord: true},
					{Old: testRewriteServerUUID, New: "00000000-0000-4000-8000-000000000001", CaseInsensitive: true},
					{Old: "AA:BB:CC:DD:EE:01", New: "02:00:00:00:00:01", CaseInsensitive: true},
					{Old: "AABBCCDDEE01", New: "020000000001", CaseInsensitive: true, WholeWord: true},
					{Old: "aa-bb-cc-dd-ee-02", New: "02:00:00:00:00:02", CaseInsensitive: true},
					{Old: "aabbccddee02", New: "020000000002", CaseInsensitive: true, WholeWord: true},
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
			got := provisioning.NewBMCRecordingRewrite(tc.server, tc.manifest, tc.operationsCenterAddress)

			// Assert
			require.Equal(t, tc.want, got, "every known value is replaced once, the first replacement of a value wins")
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
			name: "Operations Center host name from the configuration, in any case",
			line: `{"request_body":"{\"Image\":\"https://OC.corp.example.net:7443/1.0/provisioning/tokens/8F6C3D1A-2B4E-4C9A-9F7D-1A2B3C4D5E6F/seed.iso\"}"}`,

			want: `{"request_body":"{\"Image\":\"https://operations-center.example:7443/1.0/provisioning/tokens/00000000-0000-4000-8000-000000000002/seed.iso\"}"}`,
		},
		{
			name: "SKU and MAC address without separators",
			line: `"SKU":"SKU-R640-X","MAC":"aabbccddee01"`,

			want: `"SKU":"SKU000001","MAC":"020000000001"`,
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
			rewrite := provisioning.NewBMCRecordingRewrite(testRewriteServer(), testRewriteManifest(), "https://oc.corp.example.net:7443")

			// Run test
			got := rewrite.Apply(tc.line)

			// Assert
			require.Equal(t, tc.want, got, "every known value is replaced")
		})
	}
}

func TestBMCRecordingRewrite_Learn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		replacements []provisioning.BMCRecordingReplacement
		learn        []string
		lines        []string

		want []string
	}{
		{
			name: "values of identifying keys in a body are replaced on every line",
			learn: []string{
				`{"url":"https://bmc/redfish/v1/Managers/1","response_body":"{\"SerialNumber\":\"MGR0001X\",\"Oem\":{\"Dell\":{\"SystemServiceTag\":\"7QZ9999\"}},\"SKU\":\"ABCD123\"}"}`,
				`{"url":"https://bmc/redfish/v1/Managers/1/EthernetInterfaces/1","response_body":"{\"HostName\":\"idrac-7QZ9999\",\"FQDN\":\"idrac-7qz9999.corp.local\",\"PermanentMACAddress\":\"AABBCC000099\"}"}`,
			},
			lines: []string{
				`{"url":"https://idrac-7qz9999.corp.local/redfish/v1/Managers/MGR0001X","response_body":"{\"Name\":\"7QZ9999\",\"Sku\":\"ABCD123\",\"Mac\":\"aabbcc000099\",\"Host\":\"IDRAC-7QZ9999\"}"}`,
			},

			want: []string{
				`{"url":"https://host2.example/redfish/v1/Managers/SERIAL000001","response_body":"{\"Name\":\"SERVICE000001\",\"Sku\":\"SKU000001\",\"Mac\":\"020000000001\",\"Host\":\"host1.example\"}"}`,
			},
		},
		{
			name: "values in a truncated body are learned",
			learn: []string{
				`{"response_body":"{\"SerialNumber\":\"TRUNC0001\",\"Name\":\"cut her","response_truncated":true}`,
			},
			lines: []string{
				`"Serial":"TRUNC0001"`,
			},

			want: []string{
				`"Serial":"SERIAL000001"`,
			},
		},
		{
			name: "values in a base64 body are learned",
			learn: []string{
				// {"SerialNumber":"BIN00001"} followed by the byte 0xff.
				`{"response_body":"eyJTZXJpYWxOdW1iZXIiOiJCSU4wMDAwMSJ9/w==","response_body_encoding":"base64"}`,
			},
			lines: []string{
				`"Serial":"BIN00001"`,
			},

			want: []string{
				`"Serial":"SERIAL000001"`,
			},
		},
		{
			name: "explicit values keep their placeholder and the placeholder is not handed out again",
			replacements: []provisioning.BMCRecordingReplacement{
				{Old: "KNOWN0001", New: "SERIAL000001"},
			},
			learn: []string{
				`{"SerialNumber":"KNOWN0001","Chassis":{"SerialNumber":"OTHER0001"}}`,
			},
			lines: []string{
				`"a":"KNOWN0001","b":"OTHER0001"`,
			},

			want: []string{
				`"a":"SERIAL000001","b":"SERIAL000002"`,
			},
		},
		{
			name: "values that identify nothing are not learned",
			learn: []string{
				`{"SerialNumber":"Not Specified","AssetTag":"None","SKU":"123","HostName":"localhost","ServiceTag":"0000000000","FQDN":"","PartNumber":"0X3D66"}`,
			},
			lines: []string{
				`"a":"Not Specified","b":"None","c":123,"d":"localhost","e":"0000000000","f":"0X3D66"`,
			},

			want: []string{
				`"a":"Not Specified","b":"None","c":123,"d":"localhost","e":"0000000000","f":"0X3D66"`,
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
			for _, line := range tc.learn {
				rewrite.Learn(line)
			}

			got := make([]string, 0, len(tc.lines))
			for _, line := range tc.lines {
				got = append(got, rewrite.Apply(line))
			}

			// Assert
			require.Equal(t, tc.want, got, "a learned value is replaced on every line")
		})
	}
}

func TestBMCRecordingRewrite_LearnAfterApply(t *testing.T) {
	t.Parallel()

	// Setup
	rewrite := &provisioning.BMCRecordingRewrite{}
	rewrite.Apply(`"a":"LATE00001"`)

	// Run test
	rewrite.Learn(`{"SerialNumber":"LATE00001"}`)
	got := rewrite.Apply(`"a":"LATE00001"`)

	// Assert
	require.Equal(t, `"a":"LATE00001"`, got, "the rewrite is compiled on the first apply")
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
			name: "IPv4 addresses that identify nothing stay",
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
			name: "IPv4 address like version stays",
			lines: []string{
				`{"response_body":"{\"FirmwareVersion\":\"1.30.20.10\",\"Address\":\"1.30.20.10\"}"}`,
				`  "bmc_firmware_version": "4.40.10.00",`,
			},

			want: []string{
				`{"response_body":"{\"FirmwareVersion\":\"1.30.20.10\",\"Address\":\"203.0.113.1\"}"}`,
				`  "bmc_firmware_version": "4.40.10.00",`,
			},
		},
		{
			name: "IPv6 addresses are mapped stably",
			lines: []string{
				`{"response_body":"{\"IPv6Addresses\":[{\"Address\":\"fe80::aabb:ccff:fedd:eeff\"},{\"Address\":\"2a02:1234:5678::10\"}],\"Gateway\":\"FE80::AABB:CCFF:FEDD:EEFF\"}"}`,
				`{"url":"https://[2a02:1234:5678:0::10]:443/redfish/v1","mapped":"::ffff:10.0.0.7"}`,
			},

			want: []string{
				`{"response_body":"{\"IPv6Addresses\":[{\"Address\":\"2001:db8::1\"},{\"Address\":\"2001:db8::2\"}],\"Gateway\":\"2001:db8::1\"}"}`,
				`{"url":"https://[2001:db8::2]:443/redfish/v1","mapped":"2001:db8::3"}`,
			},
		},
		{
			name: "text that looks like an IPv6 address stays",
			lines: []string{
				`"Time":"2026-10-27T08:15:00Z","Loopback":"::1","Any":"::","Multicast":"ff02::1","Class":"Dell::Bar","Ratio":"1:2:3"`,
			},

			want: []string{
				`"Time":"2026-10-27T08:15:00Z","Loopback":"::1","Any":"::","Multicast":"ff02::1","Class":"Dell::Bar","Ratio":"1:2:3"`,
			},
		},
		{
			name: "values with JSON escaping in a body",
			replacements: []provisioning.BMCRecordingReplacement{
				{Old: `R&D "07"`, New: "ASSET000001"},
				{Old: `C:\path`, New: "PATH000001"},
			},
			lines: []string{
				`{"response_body":"{\"AssetTag\":\"R\\u0026D \\\"07\\\"\",\"Path\":\"C:\\\\path\"}"}`,
				`{"response_body":"{\"AssetTag\":\"R\u0026D \\\"07\\\"\"}"}`,
				`  "server_asset_tag": "R\u0026D \"07\"",`,
			},

			want: []string{
				`{"response_body":"{\"AssetTag\":\"ASSET000001\",\"Path\":\"PATH000001\"}"}`,
				`{"response_body":"{\"AssetTag\":\"ASSET000001\"}"}`,
				`  "server_asset_tag": "ASSET000001",`,
			},
		},
		{
			name: "base64 body is rewritten",
			replacements: []provisioning.BMCRecordingReplacement{
				{Old: "BIN00001", New: "SERIAL000001"},
			},
			lines: []string{
				// {"SerialNumber":"BIN00001"} followed by the byte 0xff.
				`{"sequence":1,"timestamp":"0001-01-01T00:00:00Z","duration":0,"deployment_state":"","step_retries":0,"attempt":0,"method":"GET","url":"https://10.1.2.3/x","request_headers":null,"request_body":"","request_size":0,"status":200,"response_headers":null,"response_body":"eyJTZXJpYWxOdW1iZXIiOiJCSU4wMDAwMSJ9/w==","response_body_encoding":"base64","response_size":28}`,
			},

			want: []string{
				// {"SerialNumber":"SERIAL000001"} followed by the byte 0xff.
				`{"sequence":1,"timestamp":"0001-01-01T00:00:00Z","duration":0,"deployment_state":"","step_retries":0,"attempt":0,"method":"GET","url":"https://203.0.113.1/x","request_headers":null,"request_body":"","request_size":0,"status":200,"response_headers":null,"response_body":"eyJTZXJpYWxOdW1iZXIiOiJTRVJJQUwwMDAwMDEiff8=","response_body_encoding":"base64","response_size":28}`,
			},
		},
		{
			name: "base64 body is not changed by the text pass",
			replacements: []provisioning.BMCRecordingReplacement{
				{Old: "eyJTZXJp", New: "X"},
			},
			lines: []string{
				`{"sequence":1,"timestamp":"0001-01-01T00:00:00Z","duration":0,"deployment_state":"","step_retries":0,"attempt":0,"method":"GET","url":"/eyJTZXJp","request_headers":null,"request_body":"","request_size":0,"status":200,"response_headers":null,"response_body":"eyJTZXJpYWxOdW1iZXIiOiJCSU4wMDAwMSJ9/w==","response_body_encoding":"base64","response_size":28}`,
			},

			want: []string{
				`{"sequence":1,"timestamp":"0001-01-01T00:00:00Z","duration":0,"deployment_state":"","step_retries":0,"attempt":0,"method":"GET","url":"/X","request_headers":null,"request_body":"","request_size":0,"status":200,"response_headers":null,"response_body":"eyJTZXJpYWxOdW1iZXIiOiJCSU4wMDAwMSJ9/w==","response_body_encoding":"base64","response_size":28}`,
			},
		},
		{
			name: "base64 exchange keeps no header with the original name",
			replacements: []provisioning.BMCRecordingReplacement{
				{Old: "CN7792171D0074", New: "SERIAL000001"},
			},
			lines: []string{
				`{"sequence":1,"timestamp":"0001-01-01T00:00:00Z","duration":0,"deployment_state":"","step_retries":0,"attempt":0,"method":"GET","url":"/x","request_headers":null,"request_body":"","request_size":0,"status":200,"response_headers":{"X-Serial-CN7792171D0074":["CN7792171D0074"]},"response_body":"/w==","response_body_encoding":"base64","response_size":1}`,
			},

			want: []string{
				`{"sequence":1,"timestamp":"0001-01-01T00:00:00Z","duration":0,"deployment_state":"","step_retries":0,"attempt":0,"method":"GET","url":"/x","request_headers":null,"request_body":"","request_size":0,"status":200,"response_headers":{"X-Serial-SERIAL000001":["SERIAL000001"]},"response_body":"/w==","response_body_encoding":"base64","response_size":1}`,
			},
		},
		{
			name: "base64 body that cannot be decoded is redacted",
			lines: []string{
				`{"sequence":1,"timestamp":"0001-01-01T00:00:00Z","duration":0,"deployment_state":"","step_retries":0,"attempt":0,"method":"GET","url":"/x","request_headers":null,"request_body":"","request_size":0,"status":200,"response_headers":null,"response_body":"not base64!","response_body_encoding":"base64","response_size":28}`,
			},

			want: []string{
				`{"sequence":1,"timestamp":"0001-01-01T00:00:00Z","duration":0,"deployment_state":"","step_retries":0,"attempt":0,"method":"GET","url":"/x","request_headers":null,"request_body":"","request_size":0,"status":200,"response_headers":null,"response_body":"\u003credacted\u003e","response_size":28}`,
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
