# System settings

Several global settings are available to be configured in Operations Center:

## Network settings

| Configuration         | Description                                                              | Value(s)          | Default                       |
| :---                  | :---                                                                     | :---              | :---                          |
| `address`             | Address of Operations Center which is used by managed servers to connect | https:\/\/address | same as `rest_server_address` |
| `rest_server_address` | Address/port over which the REST API will be served                      | address:port      | `*:7443`                      |

## Security settings

| Configuration                          | Description                                                              | Value(s)          | Default |
| :---                                   | :---                                                                     | :---              | :---    |
| `trusted_tls_client_cert_fingerprints` | List of SHA256 certificate fingerprints belonging to trusted TLS clients | list of strings   |         |
| `oidc`                                 | OIDC configuration                                                       |                   |         |
| `openfga`                              | OpenFGA configuration                                                    |                   |         |
| `acme`                                 | ACME certificate renewal configuration                                   |                   |         |

### OIDC

| Configuration    | Description                                                | Value(s) | Default |
| :---             | :---                                                       | :---     | :---    |
| `oidc.issuer`    | OIDC issuer                                                | string   |         |
| `oidc.client_id` | OIDC client ID used for communication with OIDC issuer     | string   |         |
| `oidc.scope`     | Scopes to be requested                                     | string   |         |
| `oidc.audience`  | Audience the OIDC tokens should be verified against        | string   |         |
| `oidc.claim`     | Claim which should be used to identify the user or subject | string   |         |

### OpenFGA

| Configuration | Description                                              | Value(s) | Default |
| :---          | :---                                                     | :---     | :---    |
| `api_token`   | API token used for communication with the OpenFGA system | string   |         |
| `api_url`     | URL of the OpenFGA API                                   | string   |         |
| `store_id`    | ID of the OpenFGA store                                  | string   |         |

### ACME

Certificate renewal will be re-attempted every 24 hours, The certificate will be replaced if there are fewer than 30 days remaining until expiry.

| Configuration             | Description                                                         | Value(s)          | Default                                          |
| :---                      | :---                                                                | :---              | :---                                             |
|  `agree_tos`              | Agree to ACME terms of service.                                     | true/false        | false                                            |
|  `ca_url`                 | URL to the directory resource of the ACME service.                  | string            | `https://acme-v02.api.letsencrypt.org/directory` |
|  `challenge`              | ACME challenge type to use.                                         | HTTP-01 or DNS-01 | `HTTP-01`                                        |
|  `domain`                 | Domain for which the certificate is issued.                         | string            |                                                  |
|  `email`                  | Email address used for the account registration.                    | string            |                                                  |
|  `http_challenge_address` | Address and interface for HTTP server (used by HTTP-01).            | string            | `:80`                                            |
|  `provider`               | Backend provider for the challenge (used by DNS-01).                | string            |                                                  |
|  `provider_environment`   | Environment variables to set during the challenge (used by DNS-01). | list of strings   |                                                  |
|  `provider_resolvers`     | List of DNS resolvers (used by DNS-01).                             | list of strings   |                                                  |

```{note}
Renewal of ACME certificates after a change of the configuration is happening
asynchronously in the background. It may take some time until the new
certificates are available.
```

## System settings

| Configuration                   | Description                                                                                                   | Value(s) | Default |
| :---                            | :---                                                                                                          | :---     | :---    |
| `log_level`                     | Log level for Operations Center logs                                                                          | string   | `INFO`  |
| `server_registration_scriptlet` | Scriptlet which is executed during server registration, see *Server registration scriptlet* below for details | string   |         |

### Server registration scriptlet

The server registration scriptlet is a [Starlark language](https://github.com/google/starlark-go/blob/master/doc/spec.md)
(which is a subset of Python) scriptlet which is executed during server
registration. It can be used to set properties of the registered server and
perform additional actions against the server.

The entry point for the server registration scriptlet is the
`server_registration` function which takes a single argument [`server`](https://github.com/FuturFusion/operations-center/blob/9cea2aecd9b387afb0e753f710a2b642956164e0/internal/provisioning/server_model.go#L22-L43)
which represents the server being registered.

Example:

```starlark
def server_registration(server):
  set_server_description("some description")
```

The following functions are available to be used in the server registration
scriptlet:

| Function                                    | Description |
| :---                                        | :---        |
| `execute_command(resource, action, body)`   | Execute a command on the server being registered. Resource is a string which identifies the resource to execute the command on (e.g. `storag`"). Use empty string (`""`) for root level commands like `reboot`. Action is a string which identifies the action to be executed (e.g. `reboot` or `scrub-pool`) The colon (`:`) prefix present in the REST API is optional and added automatically if missing. Body is a dictionary containing the parameters for the action. See the [IncusOS API documentation](https://linuxcontainers.org/incus-os/docs/main/reference/api/#/system) for the complete list of resources, their actions and the expected parameters. |
| `get_service_config(service)`               | Get the configuration of a service on the server being registered. Service is a string which identifies the service to get the configuration of, e.g. `lvm`. See the [IncusOS API documentation](https://linuxcontainers.org/incus-os/docs/main/reference/api/#/services) for the complete list of services. |
| `get_system(resource)`                      | Get the state and configuration of a system resource from the server being registered. Resource is a string which identifies the resource to get, e.g. `kernel` or `logging`. See the [IncusOS API documentation](https://linuxcontainers.org/incus-os/docs/main/reference/api/#/system) for the complete list of resources. |
| `set_server_connection_url(connection_url)` | Set the connection URL of the server being registered. |
| `set_server_description(description)`       | Set the description of the server being registered. |
| `set_server_name(name)`                     | Set the name of the server being registered. |
| `set_server_properties(properties)`         | Set the properties of the server being registered. Properties is a dictionary of string key-value pairs of type string. |
| `set_server_update_channel(update_channel)` | Set the update channel of the server being registered. Update channel is a string which should match the name of an existing update channel. |
| `set_service_config(service, config)`       | Set the configuration of a service on the server being registered. Service is a string which identifies the service to set the configuration of, e.g. `lvm`. Configuration is the configuration dictionary expected by the respective service. See the [IncusOS API documentation](https://linuxcontainers.org/incus-os/docs/main/reference/api/#/services) for the complete list of services and their expected configuration. |
| `set_system(resource, config)`              | Set the configuration of a system resource on the server being registered. Resource is a string which identifies the resource to set, e.g. `kernel` or `logging`. Configuration is the configuration dictionary expected by the respective resource. See the [IncusOS API documentation](https://linuxcontainers.org/incus-os/docs/main/reference/api/#/system) for the complete list of resources and their expected configuration. |

Additionally to the above functions, the server registration scriptlet has
access to a set of logging functions which can be used to log messages during
server registration. These functions are:

| Function                                    | Description |
| :---                                        | :---        |
| `log_error(*messages)`                      | Add a log entry to operations-center's log at error level. `messages` is one or more message arguments.    |
| `log_info(*messages)`                       | Add a log entry to operations-center's log at info level. `messages` is one or more message arguments.     |
| `log_warn(*messages)`                       | Add a log entry to operations-center's log at warning level. `messages` is one or more message arguments.  |

## Update settings

| Configuration                    | Description                                                              | Value(s) | Default                                                     |
| :---                             | :---                                                                     | :---     | :---                                                        |
| `source`                         | Source is the URL of the origin, the updates should be fetched from      | string   | `https://images.linuxcontainers.org/os/`                    |
| `signature_verification_root_ca` | Certificate used to verify the signature of updates provided by `source` | string   | root certificate used to sign updates from default `source` |
| `filter_expression`              | Filter expression to filter updates, see [update] for details            | string   | `"stable" in upstream_channels`                             |
| `file_filter_expression`         | Filter expression to filter update files, see [update] for details       | string   | `applies_to_architecture(architecture, "x86_64")`           |
| `updates_default_channel`        | Default channel for updates, see [channel] for details                   | string   | `stable`                                                    |
| `server_default_channel`         | Default channel for servers/clusters, see [channel] for details          | string   | `stable`                                                    |
