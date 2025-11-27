# System settings

Several global settings are available to be configured in Operations Center:

## Network settings

| Configuration         | Description                                                              | Value(s)          | Default                       |
| :---                  | :---                                                                     | :---              | :---                          |
| `address`             | Address of Operations Center which is used by managed servers to connect | https:\/\/address | same as `rest_server_address` |
| `rest_server_address` | Address/port over which the REST API will be served                      | address:port      | `*:7443`                      |

## Security settings

| Configuration                          | Description                                                              | Value(s)        | Default |
| :---                                   | :---                                                                     | :---            | :---    |
| `trusted_tls_client_cert_fingerprints` | List of SHA256 certificate fingerprints belonging to trusted TLS clients | list of strings |         |
| `oidc.issuer`                          | OIDC issuer                                                              | string          |         |
| `oidc.client_id`                       | OIDC client ID used for communication with OIDC issuer                   | string          |         |
| `oidc.scope`                           | Scopes to be requested                                                   | string          |         |
| `oidc.audience`                        | Audience the OIDC tokens should be verified against                      | string          |         |
| `oidc.claim`                           | Claim which should be used to identify the user or subject               | string          |         |
| `openfga.api_token`                    | API token used for communication with the OpenFGA system                 | string          |         |
| `openfga.api_url`                      | URL of the OpenFGA API                                                   | string          |         |
| `openfga.store_id`                     | ID of the OpenFGA store                                                  | string          |         |

## Update settings

| Configuration                    | Description                                                              | Value(s) | Default                                                     |
| :---                             | :---                                                                     | :---     | :---                                                        |
| `source`                         | Source is the URL of the origin, the updates should be fetched from      | string   | `https://images.linuxcontainers.org/os/`                    |
| `signature_verification_root_ca` | Certificate used to verify the signature of updates provided by `source` | string   | root certificate used to sign updates from default `source` |
| `filter_expression`              | Filter expression to filter updates, see [update] for details            | string   | `"stable" in channels`                                      |
| `file_filter_expression`         | Filter expression to filter update files, see [update] for details       | string   | `AppliesToArchitecture("x86_64")`                           |
