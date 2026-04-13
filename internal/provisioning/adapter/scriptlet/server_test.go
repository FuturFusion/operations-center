package scriptlet_test

import (
	"bytes"
	"context"
	"testing"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incusScriptlet "github.com/lxc/incus/v6/shared/scriptlet"
	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	envMock "github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/scriptlet"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/log"
	"github.com/FuturFusion/operations-center/shared/api"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

func TestRunner_ServerRegistrationRun(t *testing.T) {
	tests := []struct {
		name                         string
		script                       string
		clientGetSystem              map[string]any
		clientGetSystemErr           error
		clientUpdateSystem           any
		clientUpdateSystemErr        error
		clientTriggerSystemAction    any
		clientTriggerSystemActionErr error
		clientGetOSService           map[string]any
		clientGetOSServiceErr        error
		clientUpdateOSService        any
		clientUpdateOSServiceErr     error

		assertSetScriptletErr require.ErrorAssertionFunc
		assertRunErr          require.ErrorAssertionFunc
		assertLog             func(t *testing.T, logBuf *bytes.Buffer)
		wantServer            provisioning.Server
	}{
		{
			name:   "success - empty script",
			script: ``,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.NoError,
			assertLog:             log.Empty,
		},
		{
			name: "success - log",
			script: `
def server_registration(server):
	log_info("some info ", 1)
	log_warn("some warning ", 2)
	log_error("some error ", 3)
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.NoError,
			assertLog: func(t *testing.T, logBuf *bytes.Buffer) {
				t.Helper()

				log.Contains("INF Server registration scriptlet: some info 1")(t, logBuf)
				log.Contains("WRN Server registration scriptlet: some warning 2")(t, logBuf)
				log.Contains("ERR Server registration scriptlet: some error 3")(t, logBuf)
			},
		},
		{
			name: "success - set_*",
			script: `
def server_registration(server):
	set_server_name("name")
	set_server_description("description")
	set_server_properties({ "key": "value", "name": server.name, "timezone": server.os_data.network.config.time.timezone })
	set_server_connection_url("https://server01", False)
	set_server_connection_url("https://server01:8443", True)
	set_server_update_channel("stable")
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.NoError,
			assertLog: func(t *testing.T, logBuf *bytes.Buffer) {
				t.Helper()

				log.Contains(`INF Server registration scriptlet assigned name for server name=name`)(t, logBuf)
				log.Contains(`INF Server registration scriptlet assigned description for server name=name description=description`)(t, logBuf)
				log.Contains(`INF Server registration scriptlet assigned properties for server name=name properties="map[key:value name:server01 timezone:UTC]"`)(t, logBuf)
				log.Contains(`INF Server registration scriptlet assigned connection URL for server name=name connection_url=https://server01:8443 public=false`)(t, logBuf)
				log.Contains(`INF Server registration scriptlet assigned connection URL for server name=name connection_url=https://server01:8443 public=true`)(t, logBuf)
				log.Contains(`INF Server registration scriptlet assigned update channel for server name=name channel=stable`)(t, logBuf)
			},
			wantServer: provisioning.Server{
				Name:        "name",
				Description: "description",
				Properties: api.ConfigMap{
					"key":      "value",
					"name":     "server01",
					"timezone": "UTC",
				},
				ConnectionURL:       "https://server01:8443",
				PublicConnectionURL: "https://server01:8443",
				Channel:             "stable",
				OSData: api.OSData{
					Network: incusosapi.SystemNetwork{
						Config: &incusosapi.SystemNetworkConfig{
							Time: &incusosapi.SystemNetworkTime{
								Timezone: "UTC",
							},
						},
					},
				},
			},
		},

		{
			name: "success - get + set_system_config",
			script: `
def server_registration(server):
	info = get_system_config("kernel")
	log_info("config.blacklist_modules[0]: ", info["config"]["blacklist_modules"][0])
	set_system_config("kernel", { "config": { "blacklist_modules": [ "bad-module"], "memory": { "persistent_hugepages": 1 }, "network": { "buffer_size": 33554432, "queuing_discipline": "fq", "tcp_congestion_algorithm": "bbr" }, "pci": { "passthrough": [ { "pci_address": "0000:04:00.0", "product_id": "1050", "vendor_id": "1af4" } ] } } })
`,
			clientGetSystem: map[string]any{
				"config": map[string]any{
					"blacklist_modules": []any{"bad-module"},
					"memory": map[string]any{
						"persistent_hugepages": 1,
					},
					"network": map[string]any{
						"buffer_size":              33554432,
						"queuing_discipline":       "fq",
						"tcp_congestion_algorithm": "bbr",
					},
					"pci": map[string]any{
						"passthrough": []any{
							map[string]any{
								"pci_address": "0000:04:00.0",
								"product_id":  "1050",
								"vendor_id":   "1af4",
							},
						},
					},
				},
				"state": map[string]any{},
			},
			clientUpdateSystem: map[string]any{
				"config": map[string]any{
					"blacklist_modules": []any{"bad-module"},
					"memory": map[string]any{
						"persistent_hugepages": int64(1),
					},
					"network": map[string]any{
						"buffer_size":              int64(33554432),
						"queuing_discipline":       "fq",
						"tcp_congestion_algorithm": "bbr",
					},
					"pci": map[string]any{
						"passthrough": []any{
							map[string]any{
								"pci_address": "0000:04:00.0",
								"product_id":  "1050",
								"vendor_id":   "1af4",
							},
						},
					},
				},
			},

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.NoError,
			assertLog: func(t *testing.T, logBuf *bytes.Buffer) {
				t.Helper()

				log.Contains(`INF Server registration scriptlet: config.blacklist_modules[0]: bad-module`)(t, logBuf)
			},
		},

		{
			name: "success - trigger_system_action",
			script: `
def server_registration(server):
	trigger_system_action("storage", "scrub-pool", { "name": "mypool" })
`,
			clientTriggerSystemAction: map[string]any{
				"name": "mypool",
			},

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.NoError,
			assertLog:             log.Empty,
		},

		{
			name: "error - invalid script",
			script: `
		`,

			assertSetScriptletErr: require.Error,
		},

		{
			name: "success - get + set_service_config",
			script: `
def server_registration(server):
	info = get_service_config("lvm")
	log_info("config.enabled: ", info["config"]["enabled"])
	set_service_config("lvm", { "config": { "enabled": True } })
`,
			clientGetOSService: map[string]any{
				"config": map[string]any{
					"enabled": true,
				},
				"state": map[string]any{},
			},
			clientUpdateOSService: map[string]any{
				"config": map[string]any{
					"enabled": true,
				},
			},

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.NoError,
			assertLog: func(t *testing.T, logBuf *bytes.Buffer) {
				t.Helper()

				log.Contains(`INF Server registration scriptlet: config.enabled: True`)(t, logBuf)
			},
		},

		{
			name: "error - scriptlet fail",
			script: `
def server_registration(server):
	fail("oops")
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Empty,
		},
		{
			name: "error - set_server_name - invalid argument count",
			script: `
def server_registration(server):
	set_server_name("foobar", "additional argument")
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Empty,
		},
		{
			name: "error - set_server_name - empty name",
			script: `
def server_registration(server):
	set_server_name("")
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Contains("ERR Server registration scriptlet failed. Server name is empty"),
		},
		{
			name: "error - set_server_description - invalid argument count",
			script: `
def server_registration(server):
	set_server_description("foobar", "additional argument")
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Empty,
		},
		{
			name: "error - set_server_properties - additional argument",
			script: `
def server_registration(server):
	set_server_properties({"key": "value"}, "additional argument")
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Empty,
		},
		{
			name: "error - set_server_properties - invalid property key type",
			script: `
def server_registration(server):
	set_server_properties({10: "value"})
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `"int" is an unexpected property key type, require "string"`)
			},
			assertLog: log.Empty,
		},
		{
			name: "error - set_server_properties - invalid property value type",
			script: `
def server_registration(server):
	set_server_properties({"key": 10})
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `"int" is an unexpected property value type, require "string"`)
			},
			assertLog: log.Empty,
		},
		{
			name: "error - set_server_connection_url - invalid argument count",
			script: `
def server_registration(server):
	set_server_connection_url("foobar", False, "additional argument")
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Empty,
		},
		{
			name: "error - set_server_connection_url - invalid connection url",
			script: `
def server_registration(server):
	set_server_connection_url(":|//", False)
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Contains("ERR Server registration scriptlet failed. Server connection URL is not valid"),
		},
		{
			name: "error - set_server_connection_url - invalid connection url schema not https",
			script: `
def server_registration(server):
	set_server_connection_url("http://server01", False)
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Contains("ERR Server registration scriptlet failed. Server connection URL, schema is not https"),
		},
		{
			name: "error - set_server_connection_url - empty connection url",
			script: `
def server_registration(server):
	set_server_connection_url("", False)
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Contains("ERR Server registration scriptlet failed. Server connection URL is empty"),
		},
		{
			name: "error - set_server_update_channel - invalid argument count",
			script: `
def server_registration(server):
	set_server_update_channel("stable", "additional argument")
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Empty,
		},

		{
			name: "error - get_system_config - invalid argument count",
			script: `
def server_registration(server):
	get_system_config()
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Empty,
		},
		{
			name: "error - get_system_config - client",
			script: `
def server_registration(server):
	get_system_config("kernel")
`,
			clientGetSystemErr: boom.Error,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          boom.ErrorIs,
			assertLog:             log.Empty,
		},
		{
			name: "error - get_system_config - starlark marshal",
			script: `
def server_registration(server):
	get_system_config("kernel")
`,
			clientGetSystem: map[string]any{
				"invalid": func() {}, // functions are invalid types for starlark marshal.
			},

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Empty,
		},
		{
			name: "error - set_system_config - invalid argument count",
			script: `
def server_registration(server):
	set_system_config("kernel", { "config": { "blacklist_modules": [ "bad-module"], "memory": { "persistent_hugepages": 1 }, "network": { "buffer_size": 33554432, "queuing_discipline": "fq", "tcp_congestion_algorithm": "bbr" }, "pci": { "passthrough": [ { "pci_address": "0000:04:00.0", "product_id": "1050", "vendor_id": "1af4" } ] } } }, "additional argument")
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Empty,
		},
		{
			name: "error - set_system_config - invalid argument - unsupported starlark.Dict",
			script: `
def server_registration(server):
	set_system_config("kernel", {1: ""})
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Empty,
		},
		{
			name: "error - set_system_config - client",
			script: `
def server_registration(server):
	set_system_config("kernel", { "config": { "blacklist_modules": [ "bad-module"], "memory": { "persistent_hugepages": 1 }, "network": { "buffer_size": 33554432, "queuing_discipline": "fq", "tcp_congestion_algorithm": "bbr" }, "pci": { "passthrough": [ { "pci_address": "0000:04:00.0", "product_id": "1050", "vendor_id": "1af4" } ] } } })
`,
			clientUpdateSystem: map[string]any{
				"config": map[string]any{
					"blacklist_modules": []any{"bad-module"},
					"memory": map[string]any{
						"persistent_hugepages": int64(1),
					},
					"network": map[string]any{
						"buffer_size":              int64(33554432),
						"queuing_discipline":       "fq",
						"tcp_congestion_algorithm": "bbr",
					},
					"pci": map[string]any{
						"passthrough": []any{
							map[string]any{
								"pci_address": "0000:04:00.0",
								"product_id":  "1050",
								"vendor_id":   "1af4",
							},
						},
					},
				},
			},
			clientUpdateSystemErr: boom.Error,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          boom.ErrorIs,
			assertLog:             log.Empty,
		},

		{
			name: "error - trigger_system_action - invalid argument count",
			script: `
def server_registration(server):
	trigger_system_action()
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Empty,
		},
		{
			name: "error - trigger_system_action - invalid argument - unsupported starlark.Dict",
			script: `
def server_registration(server):
	trigger_system_action("storage", "scrub-pool", {1: ""})
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Empty,
		},
		{
			name: "error - trigger_system_action - client",
			script: `
def server_registration(server):
	trigger_system_action("storage", "scrub-pool", { "name": "mypool" })
`,
			clientTriggerSystemAction: map[string]any{
				"name": "mypool",
			},
			clientTriggerSystemActionErr: boom.Error,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          boom.ErrorIs,
			assertLog:             log.Empty,
		},

		{
			name: "error - get_service_config - invalid argument count",
			script: `
def server_registration(server):
	get_service_config()
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Empty,
		},
		{
			name: "error - get_service_config - client",
			script: `
def server_registration(server):
	get_service_config("lvm")
`,
			clientGetOSServiceErr: boom.Error,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          boom.ErrorIs,
			assertLog:             log.Empty,
		},
		{
			name: "error - get_service_config - starlark marshal",
			script: `
def server_registration(server):
	get_service_config("lvm")
`,
			clientGetOSService: map[string]any{
				"invalid": func() {}, // functions are invalid types for starlark marshal.
			},

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Empty,
		},
		{
			name: "error - set_service_config - invalid argument count",
			script: `
def server_registration(server):
	set_service_config("lvm", { "config": { "enabled": True } }, "additional argument")
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Empty,
		},
		{
			name: "error - set_service_config - invalid argument - unsupported starlark.Dict",
			script: `
def server_registration(server):
	set_service_config("lvm", {1: ""})
`,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          require.Error,
			assertLog:             log.Empty,
		},
		{
			name: "error - set_service_config - client",
			script: `
def server_registration(server):
	set_service_config("lvm", { "config": { "enabled": True } })
`,
			clientUpdateOSService: map[string]any{
				"config": map[string]any{
					"enabled": true,
				},
			},
			clientUpdateOSServiceErr: boom.Error,

			assertSetScriptletErr: require.NoError,
			assertRunErr:          boom.ErrorIs,
			assertLog:             log.Empty,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			config.InitTest(t, &envMock.EnvironmentMock{
				IsIncusOSFunc: func() bool {
					return false
				},
			}, nil)

			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", true, false, true)
			require.NoError(t, err)

			client := &adapterMock.ScriptletClientPortMock{
				GetSystemFunc: func(ctx context.Context, server provisioning.Server, resource string) (map[string]any, error) {
					return tc.clientGetSystem, tc.clientGetSystemErr
				},
				UpdateSystemFunc: func(ctx context.Context, server provisioning.Server, resource string, config any) error {
					require.Equal(t, tc.clientUpdateSystem, config)
					return tc.clientUpdateSystemErr
				},
				TriggerSystemActionFunc: func(ctx context.Context, server provisioning.Server, resource, action string, body any) error {
					require.Equal(t, tc.clientTriggerSystemAction, body)
					return tc.clientTriggerSystemActionErr
				},
				GetOSServiceFunc: func(ctx context.Context, server provisioning.Server, name string) (map[string]any, error) {
					return tc.clientGetOSService, tc.clientGetOSServiceErr
				},
				UpdateOSServiceFunc: func(ctx context.Context, server provisioning.Server, name string, config any) error {
					require.Equal(t, tc.clientUpdateOSService, config)
					return tc.clientUpdateOSServiceErr
				},
			}

			loader := incusScriptlet.NewLoader()
			runner, err := scriptlet.New(loader, client)
			require.NoError(t, err)
			defer lifecycle.SettingsUpdateSignal.Reset()
			defer lifecycle.SettingsValidateSignal.Reset()

			// Load script
			err = config.UpdateSettings(t.Context(), system.SettingsPut{
				ServerRegistrationScriptlet: tc.script,
			})
			tc.assertSetScriptletErr(t, err)
			if err != nil {
				return
			}

			// Run test
			server := provisioning.Server{
				Name:          "server01",
				ConnectionURL: "https://1.2.3.4:8443",
				OSData: api.OSData{
					Network: incusosapi.SystemNetwork{
						Config: &incusosapi.SystemNetworkConfig{
							Time: &incusosapi.SystemNetworkTime{
								Timezone: "UTC",
							},
						},
					},
				},
			}

			err = runner.ServerRegistrationRun(t.Context(), &server)
			tc.assertRunErr(t, err)

			// Assertions
			if tc.wantServer.Name != "" {
				require.Equal(t, tc.wantServer, server)
			}

			tc.assertLog(t, logBuf)
		})
	}
}
