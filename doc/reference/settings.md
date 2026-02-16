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

## Update settings

| Configuration                    | Description                                                              | Value(s) | Default                                                     |
| :---                             | :---                                                                     | :---     | :---                                                        |
| `source`                         | Source is the URL of the origin, the updates should be fetched from      | string   | `https://images.linuxcontainers.org/os/`                    |
| `signature_verification_root_ca` | Certificate used to verify the signature of updates provided by `source` | string   | root certificate used to sign updates from default `source` |
| `filter_expression`              | Filter expression to filter updates, see [update] for details            | string   | `"stable" in upstream_channels`                             |
| `file_filter_expression`         | Filter expression to filter update files, see [update] for details       | string   | `applies_to_architecture(architecture, "x86_64")`           |
| `updates_default_channel`        | Default channel for updates, see [channel] for details                   | string   | `stable`                                                    |
| `server_default_channel`         | Default channel for servers/clusters, see [channel] for details          | string   | `stable`                                                    |
