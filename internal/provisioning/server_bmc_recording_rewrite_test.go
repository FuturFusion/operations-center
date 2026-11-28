package provisioning_test

import (
	"fmt"
	"regexp"
	"strings"
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
			"https://[fd00:1234::5]:7443/media/seed.iso",
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
					{Old: "calvin", New: "<redacted>", WholeWord: true},
					{Old: "root", New: "bmc-user", KeySuffix: "username"},
					{Old: "192.168.1.100", New: "bmc.example", CaseInsensitive: true, WholeWord: true},
					{Old: "r640-03.lab.example.net", New: "server.example", CaseInsensitive: true, WholeWord: true},
					{Old: "r640-03", New: "server01", CaseInsensitive: true, WholeWord: true},
					{Old: "oc.corp.example.net", New: "operations-center.example", CaseInsensitive: true, WholeWord: true},
					{Old: "10.0.0.5", New: "operations-center.example", CaseInsensitive: true, WholeWord: true},
					{Old: "[fd00:1234::5]", New: "operations-center.example", CaseInsensitive: true},
					{Old: "fd00:1234::5", New: "operations-center.example", CaseInsensitive: true, WholeWord: true},
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
			name: "password equal to the user name is replaced everywhere",
			server: provisioning.Server{
				BMCConfig: api.BMCConfig{Username: "admin", Password: "admin"},
			},

			want: &provisioning.BMCRecordingRewrite{
				GenericPatterns: true,
				Replacements: []provisioning.BMCRecordingReplacement{
					{Old: "admin", New: "<redacted>", WholeWord: true},
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
			name: "server name joined to other words by a dash or an underscore",
			line: `{"response_body":"{\"incus.devices.boot-media\":\"{\\\"source\\\":\\\"r640-03-boot-media.iso\\\"}\",\"Path\":\"/var/lib/media/default_r640-03/image.iso\"}"}`,

			want: `{"response_body":"{\"incus.devices.boot-media\":\"{\\\"source\\\":\\\"server01-boot-media.iso\\\"}\",\"Path\":\"/var/lib/media/default_server01/image.iso\"}"}`,
		},
		{
			name: "server name in any case",
			line: `"seed": "deploy-R640-03", "Name":"R640-03b"`,

			want: `"seed": "deploy-server01", "Name":"R640-03b"`,
		},
		{
			name: "Operations Center host name from the configuration, in any case",
			line: `{"request_body":"{\"Image\":\"https://OC.corp.example.net:7443/1.0/provisioning/tokens/8F6C3D1A-2B4E-4C9A-9F7D-1A2B3C4D5E6F/seed.iso\"}"}`,

			want: `{"request_body":"{\"Image\":\"https://operations-center.example:7443/1.0/provisioning/tokens/00000000-0000-4000-8000-000000000002/seed.iso\"}"}`,
		},
		{
			name: "IPv6 address of Operations Center, the URL stays valid",
			line: `{"request_body":"{\"Image\":\"https://[FD00:1234::5]:7443/media/seed.iso\"}","MessageArgs":["fd00:1234::5"]}`,

			want: `{"request_body":"{\"Image\":\"https://operations-center.example:7443/media/seed.iso\"}","MessageArgs":["operations-center.example"]}`,
		},
		{
			name: "user name only as the value of a user name key",
			line: `{"response_body":"{\"UserName\":\"root\",\"RoleId\":\"root\",\"Message\":\"root logged in\"}","manifest":{"LoginUsername": "root"}}`,

			want: `{"response_body":"{\"UserName\":\"bmc-user\",\"RoleId\":\"root\",\"Message\":\"root logged in\"}","manifest":{"LoginUsername": "bmc-user"}}`,
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
			name: "serial numbers of certificates are not learned",
			learn: []string{
				`{"response_body":"{\"Members\":[{\"SerialNumber\":\"5DA7661FCB4B26D3\",\"CertificateString\":\"-----BEGIN CERTIFICATE-----\"}]}"}`,
				`{"response_body":"{\"SerialNumber\":\"CHASSIS0001\"}"}`,
			},
			lines: []string{
				`"a":"5DA7661FCB4B26D3","b":"CHASSIS0001"`,
			},

			want: []string{
				`"a":"5DA7661FCB4B26D3","b":"SERIAL000001"`,
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
				`{"response_body":"{\"TpmFirmware\":\"1.3.2.8\",\"HardwareRevision\":\"2.1.0.3\"}"}`,
			},

			want: []string{
				`{"response_body":"{\"FirmwareVersion\":\"1.30.20.10\",\"Address\":\"203.0.113.1\"}"}`,
				`  "bmc_firmware_version": "4.40.10.00",`,
				`{"response_body":"{\"TpmFirmware\":\"1.3.2.8\",\"HardwareRevision\":\"2.1.0.3\"}"}`,
			},
		},
		{
			name: "IPv6 addresses are mapped stably",
			lines: []string{
				`{"response_body":"{\"IPv6Addresses\":[{\"Address\":\"fe80::aabb:ccff:fedd:eeff\"},{\"Address\":\"fd00:1234:5678::10\"}],\"Gateway\":\"FE80::AABB:CCFF:FEDD:EEFF\"}"}`,
				`{"url":"https://[fd00:1234:5678:0::10]:443/redfish/v1","mapped":"::ffff:10.0.0.7"}`,
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

// testRewriteRecording holds lines of real recordings of Dell iDRAC 9, HPE
// iLO 5 and Lenovo XCC, with values that the anonymizer missed before.
var testRewriteRecording = []string{
	`  "started_at": "2026-01-15T10:20:30.267428567+01:00",`,
	`  "media_urls": ["https://10.0.0.5:7443/1.0/provisioning/tokens/8f6c3d1a-2b4e-4c9a-9f7d-1a2b3c4d5e6f/seeds/r640-03/architecture/x86_64/deployment/aB3dE5fG7hJ9/type/iso/kL2mN4pQ6rS8.iso", "https://10.0.0.5:7443/1.0/provisioning/secure-boot-media/q8Xz_4-kLm0N.iso"],`,
	`{"url":"https://192.168.1.100:8443/redfish/v1/Systems/1","response_body":"{\"Boot\":{\"UefiTargetBootSourceOverride@Redfish.AllowableValues\":[\"PciRoot(0x0)/Pci(0x1,0x1)/Pci(0x0,0x0)/MAC(AABBCC001010,0x1)/IPv4(0.0.0.0)\",\"PciRoot(0x0)/Pci(0x1,0x1)/Pci(0x0,0x0)/MAC(AABBCC001011,0x1)/IPv6(fe80::a8bb:ccff:fe00:1011)\"]}}"}`,
	`{"url":"https://192.168.1.100:8443/redfish/v1/Managers/1","response_body":"{\"TimeZoneName\":\"JST-9\",\"DateTimeLocalOffset\":\"+09:00\",\"DateTime\":\"2026-01-15T18:20:33+09:00\",\"LastResetTime\":\"2026-01-10T08:00:00.123+10:00\",\"Oem\":{\"Hpe\":{\"License\":{\"LicenseKey\":\"XXXXX-XXXXX-XXXXX-XXXXX-Z9Y8W\"}}}}"}`,
	`{"method":"POST","request_body":"{\"Image\":\"https://10.0.0.5:7443/1.0/provisioning/tokens/8f6c3d1a-2b4e-4c9a-9f7d-1a2b3c4d5e6f/seeds/r640-03/architecture/x86_64/deployment/aB3dE5fG7hJ9/type/iso/kL2mN4pQ6rS8.iso\"}","response_body":"{\"Message\":\"Unable to mount https://10.0.0.5:7443/1.0/provisioning/secure-boot-media/q8Xz_4-kLm0N.iso\"}"}`,
	`{"response_body":"{\"Image\":\"https://10.0.0.5:7443/1.0/provisioning/secure-boot-media/q8Xz_4-kLm0N.iso\",\"ImageName\":\"q8Xz_4-kLm0N.iso\"}"}`,
	`{"response_body":"{\"Attributes\":{\"TpmFirmware\":\"7.2.2.0\",\"MicrocodeRev\":\"1.2.3.4\",\"IPv4Address\":\"192.168.1.20\"}}"}`,
}

func TestBMCRecordingRewrite_Anonymize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		lines []string

		want []string
	}{
		{
			name: "MAC address without separators in a UEFI device path",
			lines: []string{
				`{"response_body":"{\"AllowableValues\":[\"PciRoot(0x0)/Pci(0x0,0x0)/MAC(AABBCC001010,0x1)/IPv4(0.0.0.0)\",\"PciRoot(0x0)/MAC(aabbcc001011)\"],\"MACAddress\":\"aa:bb:cc:00:10:11\"}"}`,
				`"MAC":"AA-BB-CC-00-10-10"`,
			},

			want: []string{
				`{"response_body":"{\"AllowableValues\":[\"PciRoot(0x0)/Pci(0x0,0x0)/MAC(020000000002,0x1)/IPv4(0.0.0.0)\",\"PciRoot(0x0)/MAC(020000000001)\"],\"MACAddress\":\"02:00:00:00:00:01\"}"}`,
				`"MAC":"02:00:00:00:00:02"`,
			},
		},
		{
			name: "unique values in other UEFI device path nodes",
			lines: []string{
				`"a":"PciRoot(0x0)/Pci(0x1,0x0)/Fibre(0x1122334455667788,0x0)","b":"PciRoot(0x0)/Pci(0x2,0x0)/NVMe(0x1,00-11-22-33-44-55-66-77)"`,
				`"c":"MAC(AABBCC001010,0x1)/IPv4(192.168.1.20,TCP,DHCP,192.168.1.21,192.168.1.1,255.255.255.0)/Uri(http://boot.corp.lan/boot.efi)"`,
				`"d":"MAC(AABBCC001010,0x1)/IPv6(fd00:1234::20,TCP,StatefulAutoConfigure,fd00:1234::21,64,fd00:1234::1)/Uri(https://[fd00:1234::22]/boot.efi)"`,
				`"e":"boot.corp.lan","f":"00:11:22:33:44:55:66:77"`,
			},

			want: []string{
				`"a":"PciRoot(0x0)/Pci(0x1,0x0)/Fibre(0x5000000000000001,0x0)","b":"PciRoot(0x0)/Pci(0x2,0x0)/NVMe(0x1,02-00-00-00-00-00-00-01)"`,
				`"c":"MAC(020000000001,0x1)/IPv4(203.0.113.1,TCP,DHCP,203.0.113.2,203.0.113.3,255.255.255.0)/Uri(http://host1.example/boot.efi)"`,
				`"d":"MAC(020000000001,0x1)/IPv6(2001:db8::1,TCP,StatefulAutoConfigure,2001:db8::2,64,2001:db8::3)/Uri(https://[2001:db8::4]/boot.efi)"`,
				`"e":"host1.example","f":"02:00:00:00:00:00:00:01"`,
			},
		},
		{
			name: "HPE iLO license key by key and by value",
			lines: []string{
				`{"response_body":"{\"Oem\":{\"Hpe\":{\"License\":{\"LicenseKey\":\"XXXXX-XXXXX-XXXXX-XXXXX-Z9Y8W\"}}}}"}`,
				`{"response_body":"{\"LicenseKey\":\"ab12-CD34\",\"Message\":\"License 34RTZ-AB12C-9Q8WE-ZU7IO-Z9Y8W installed\"}"}`,
			},

			want: []string{
				`{"response_body":"{\"Oem\":{\"Hpe\":{\"License\":{\"LicenseKey\":\"XXXXX-XXXXX-XXXXX-XXXXX-XXXXX\"}}}}"}`,
				`{"response_body":"{\"LicenseKey\":\"XXXX-XXXX\",\"Message\":\"License XXXXX-XXXXX-XXXXX-XXXXX-XXXXX installed\"}"}`,
			},
		},
		{
			name: "time zone and timestamps with an offset",
			lines: []string{
				`{"timestamp":"2026-01-15T10:20:30.5+01:00","response_body":"{\"TimeZoneName\":\"JST-9\",\"DateTimeLocalOffset\":\"+09:00\",\"DateTime\":\"2026-01-15T18:20:33+09:00\",\"LastResetTime\":\"2026-01-10T08:00:00.123+10:00\"}"}`,
				`  "started_at": "2026-01-15T10:20:30.267428567+01:00", "finished_at": "2026-01-15T09:50:30Z", "zero": "2026-01-15T09:50:30-00:00",`,
				`{"response_body":"{\"Offset\":\"+09:00\",\"Zone\":\"JST-9\",\"Valid\":\"2020-01-01T00:00:00+00:00\"}"}`,
			},

			want: []string{
				`{"timestamp":"2026-01-15T09:20:30.5+00:00","response_body":"{\"TimeZoneName\":\"Etc/UTC\",\"DateTimeLocalOffset\":\"+00:00\",\"DateTime\":\"2026-01-15T09:20:33+00:00\",\"LastResetTime\":\"2026-01-09T22:00:00.123+00:00\"}"}`,
				`  "started_at": "2026-01-15T09:20:30.267428567+00:00", "finished_at": "2026-01-15T09:50:30Z", "zero": "2026-01-15T09:50:30+00:00",`,
				`{"response_body":"{\"Offset\":\"+09:00\",\"Zone\":\"JST-9\",\"Valid\":\"2020-01-01T00:00:00+00:00\"}"}`,
			},
		},
		{
			name: "IDs in the media URLs of Operations Center",
			lines: []string{
				`  "media_urls": ["https://oc.example/1.0/provisioning/tokens/t/seeds/s/architecture/x86_64/deployment/aB3dE5fG7hJ9/type/iso/kL2mN4pQ6rS8.iso", "https://oc.example/1.0/provisioning/secure-boot-media/q8Xz_4-kLm0N.iso"],`,
				`{"request_body":"{\"Image\":\"https://oc.example/1.0/provisioning/tokens/t/seeds/s/architecture/x86_64/deployment/aB3dE5fG7hJ9/type/iso/kL2mN4pQ6rS8.iso\"}","response_body":"{\"Message\":\"Unable to mount https://oc.example/1.0/provisioning/secure-boot-media/q8Xz_4-kLm0N.iso\"}"}`,
				`{"response_body":"{\"Image\":\"https://oc.example/1.0/provisioning/secure-boot-media/q8Xz_4-kLm0N.iso\",\"ImageName\":\"q8Xz_4-kLm0N.iso\",\"Other\":\"kL2mN4pQ6rS8.iso\"}"}`,
				`  "image_name": "kL2mN4pQ6rS8.iso", "file": "file.iso"`,
			},

			want: []string{
				`  "media_urls": ["https://oc.example/1.0/provisioning/tokens/t/seeds/s/architecture/x86_64/deployment/DEPLOYMENT01/type/iso/SEEDIMAGE001.iso", "https://oc.example/1.0/provisioning/secure-boot-media/SBMEDIA00001.iso"],`,
				`{"request_body":"{\"Image\":\"https://oc.example/1.0/provisioning/tokens/t/seeds/s/architecture/x86_64/deployment/DEPLOYMENT01/type/iso/SEEDIMAGE001.iso\"}","response_body":"{\"Message\":\"Unable to mount https://oc.example/1.0/provisioning/secure-boot-media/SBMEDIA00001.iso\"}"}`,
				`{"response_body":"{\"Image\":\"https://oc.example/1.0/provisioning/secure-boot-media/SBMEDIA00001.iso\",\"ImageName\":\"SBMEDIA00001.iso\",\"Other\":\"SEEDIMAGE001.iso\"}"}`,
				`  "image_name": "SEEDIMAGE001.iso", "file": "file.iso"`,
			},
		},
		{
			name: "versions that look like an IPv4 address stay",
			lines: []string{
				`{"response_body":"{\"Attributes\":{\"Tpm2Hierarchy\":\"Enabled\",\"TpmFirmware\":\"7.2.2.0\",\"CpuMicrocode\":\"1.2.3.4\",\"MicrocodeRev\":\"1.2.3.5\",\"IPv4Address\":\"7.2.2.0\"}}"}`,
				`      "TpmFirmware": "7.2.2.0",`,
			},

			want: []string{
				`{"response_body":"{\"Attributes\":{\"Tpm2Hierarchy\":\"Enabled\",\"TpmFirmware\":\"7.2.2.0\",\"CpuMicrocode\":\"1.2.3.4\",\"MicrocodeRev\":\"1.2.3.5\",\"IPv4Address\":\"203.0.113.1\"}}"}`,
				`      "TpmFirmware": "7.2.2.0",`,
			},
		},
		{
			name: "address under a key that contains a version word is replaced",
			lines: []string{
				`{"response_body":"{\"FirmwareUpdateServerAddress\":\"7.2.2.0\",\"FirmwareVersion\":\"7.2.2.0\"}"}`,
			},

			want: []string{
				`{"response_body":"{\"FirmwareUpdateServerAddress\":\"203.0.113.1\",\"FirmwareVersion\":\"7.2.2.0\"}"}`,
			},
		},
		{
			name: "long hex sequence is replaced, except as a certificate fingerprint",
			lines: []string{
				`{"response_body":"{\"Data\":\"1A:2B:3C:4D:5E:6F:70:81:92:A3\",\"Other\":\"1a-2b-3c-4d-5e-6f-70-81-92-a4\",\"Again\":\"1a:2b:3c:4d:5e:6f:70:81:92:a3\",\"Thumbprint\":\"1A:2B:3C:4D:5E:6F:70:81:92:A3\"}"}`,
			},

			want: []string{
				`{"response_body":"{\"Data\":\"00:00:00:00:00:00:00:00:00:01\",\"Other\":\"00-00-00-00-00-00-00-00-00-02\",\"Again\":\"00:00:00:00:00:00:00:00:00:01\",\"Thumbprint\":\"1A:2B:3C:4D:5E:6F:70:81:92:A3\"}"}`,
			},
		},
		{
			name: "serial number of a certificate stays, also if seen before the certificate",
			lines: []string{
				`{"response_body":"{\"SerialNumber\":\"330000001A2B3C\"}"}`,
				`{"response_body":"{\"CertificateString\":\"-----BEGIN CERTIFICATE-----\\nMIIF\\n-----END CERTIFICATE-----\",\"SerialNumber\":\"330000001A2B3C\",\"Fingerprint\":\"1A:2B:3C:4D:5E:6F:70:81:92:A3:B4:C5\"}"}`,
			},

			want: []string{
				`{"response_body":"{\"SerialNumber\":\"330000001A2B3C\"}"}`,
				`{"response_body":"{\"CertificateString\":\"-----BEGIN CERTIFICATE-----\\nMIIF\\n-----END CERTIFICATE-----\",\"SerialNumber\":\"330000001A2B3C\",\"Fingerprint\":\"1A:2B:3C:4D:5E:6F:70:81:92:A3:B4:C5\"}"}`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			rewrite := &provisioning.BMCRecordingRewrite{GenericPatterns: true}
			for _, line := range tc.lines {
				rewrite.Learn(line)
			}

			// Run test
			got := make([]string, 0, len(tc.lines))
			for _, line := range tc.lines {
				got = append(got, rewrite.Apply(line))
			}

			// Assert
			require.Equal(t, tc.want, got, "every identifying value is replaced and equal values stay equal")
		})
	}
}

func anonymizeTestRecording() []string {
	rewrite := provisioning.NewBMCRecordingRewrite(testRewriteServer(), testRewriteManifest(), "https://oc.corp.example.net:7443")
	for _, line := range testRewriteRecording {
		rewrite.Learn(line)
	}

	got := make([]string, 0, len(testRewriteRecording))
	for _, line := range testRewriteRecording {
		got = append(got, rewrite.Apply(line))
	}

	return got
}

func TestBMCRecordingRewrite_StablePlaceholders(t *testing.T) {
	t.Parallel()

	// Run test
	first := anonymizeTestRecording()
	second := anonymizeTestRecording()

	// Assert
	require.NotEqual(t, testRewriteRecording, first, "the recording is anonymized")
	require.Equal(t, first, second, "two runs over the same recording use the same placeholders")
}

func TestBMCRecordingRewrite_NoLeaks(t *testing.T) {
	t.Parallel()

	// These patterns come from profiles/anonymization_test.go of
	// incus-redfish-proxy.
	checks := []struct {
		name    string
		find    *regexp.Regexp
		allowed *regexp.Regexp
	}{
		{
			name:    "MAC address",
			find:    regexp.MustCompile(`(?i)\b[0-9a-f]{2}(?:[:-][0-9a-f]{2}){5}\b`),
			allowed: regexp.MustCompile(`^02[:-]00[:-]00[:-]00[:-]00[:-][0-9A-Fa-f]{2}$`),
		},
		{
			name:    "MAC address in a UEFI device path",
			find:    regexp.MustCompile(`MAC\([0-9A-Fa-f]+`),
			allowed: regexp.MustCompile(`^MAC\(0200000000[0-9A-Fa-f]{2}$`),
		},
		{
			name:    "time zone offset",
			find:    regexp.MustCompile(`\d{2}:\d{2}(?::\d{2}(?:\.\d+)?)?[+-]\d{2}:\d{2}\b|"[+-]\d{2}:\d{2}\\?"`),
			allowed: regexp.MustCompile(`^(?:\d{2}:\d{2}(?::\d{2}(?:\.\d+)?)?\+00:00|"\+00:00\\?")$`),
		},
		{
			name:    "license key",
			find:    regexp.MustCompile(`\b[0-9A-Z]{5}(?:-[0-9A-Z]{5}){4}\b`),
			allowed: regexp.MustCompile(`^XXXXX(?:-XXXXX){4}$`),
		},
		{
			name:    "media URL ID",
			find:    regexp.MustCompile(`[A-Za-z0-9_-]{12}\.(?:iso|raw|img)\b|deployment/[A-Za-z0-9_-]+`),
			allowed: regexp.MustCompile(`^(?:(?:SEEDIMAGE\d{3}|SBMEDIA\d{5})\.(?:iso|raw|img)|deployment/DEPLOYMENT\d{2})$`),
		},
	}

	// Run test
	got := anonymizeTestRecording()

	// Assert
	for _, line := range got {
		for _, check := range checks {
			for _, match := range check.find.FindAllString(line, -1) {
				require.Regexp(t, check.allowed, match, "%s is anonymized in %s", check.name, line)
			}
		}
	}
}

func TestBMCRecordingRewrite_PlaceholdersRunOut(t *testing.T) {
	t.Parallel()

	// Setup
	macs := make([]string, 0, 0x10000)
	for n := range 0x10000 {
		macs = append(macs, fmt.Sprintf("aa:bb:cc:dd:%02x:%02x", n>>8, n&0xff))
	}

	rewrite := &provisioning.BMCRecordingRewrite{GenericPatterns: true}

	// Run test
	got := strings.Split(rewrite.Apply(strings.Join(macs, " ")), " ")

	// Assert
	require.Equal(t, []string{"02:00:00:00:ff:fe", "02:00:00:00:ff:ff", "<redacted>"}, got[len(got)-3:], "a value is redacted once the placeholders of its kind run out")
}
