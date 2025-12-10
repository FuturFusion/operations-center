# Accessing the system

## Local access (command line)

By default, the `operations-center` CLI tool can be used to manage an Operations
Center service running on the same system.

## Network settings (command line)

To enable `operations-center` to communicate over the network, you can assign a
network address and port. If no port is specified, Operations Center will use
port `7443` by default if run stand alone and `8443` if run as application
on top of IncusOS.

```shell
$ operations-center system network edit

### This is a YAML representation of the network configuration.
### Any line starting with a '# will be ignored.

address: https://example.com:443
rest_server_address: '[192.0.2.100]:443'

```

If `rest_server_address` is set, `address` needs to be set as well.

## Security settings (command line)

Authentication and authorization settings can be configured from the command
line as well. Operations Center will only accept trusted connections.

```shell
$ operations-center system security edit

### This is a YAML representation of the security configuration.
### Any line starting with a '# will be ignored.

trusted_tls_client_cert_fingerprints:
    - e385d0e91509d33f0a3ff2d5993bd1fc6e6265140b5f11b7e3d20801480e3fbf
    - a57be4e28ab1f1d315e9d3b174a54221b47dca44f2e5c7c436d9cf558e3f8b7e
oidc:
    issuer: ""
    client_id: ""
    scopes: ""
    audience: ""
    claim: ""
openfga:
    api_token: ""
    api_url: ""
    store_id: ""

```

## Remote access (command line)

The CLI tool can connect to an Operations Center service over the network by
registering a remote. Here is a sample registration of a remote named `m1` at
address `https://192.0.2.100:443`:

```shell
$ operations-center remote add "m1" "https://192.0.2.100:443" --auth-type "tls"
Server presented an untrusted TLS certificate with SHA256 fingerprint 80d569e9244a421f3a3d60d46631eb717f8a0a480f2f23ee729a4c1c016875f7. Is this the correct fingerprint? (yes/no) [default=no]: yes

$ operations-center remote switch "m1"
```

Additionally, `--auth-type "oidc"` is available if configured on the Operations
Center service.

The first time the remote CLI tool is used, a certificate keypair will be
generated that must be trusted by the Operations Center service:

```text
Received authentication mismatch: got "untrusted", expected "tls". Ensure the server trusts the client fingerprint "653f014cbd7a7135c21414884283a50f2dd8e117943e4593638d72824596b268"
```

This certificate should be added to the `trusted_tls_client_cert_fingerprints` list with the local CLI tool using `operations-center system security edit` for the remote CLI to properly function.

## From the web

The Operations Center UI is also available for web access.

For this to work, a client certificate trusted by Operations Center must be
imported as a user certificate in your web browser. The remote CLI keypair
generated in `~/.config/operations-center` can be used for this purpose.

The exact process to do this varies between browsers and operating
systems, but generally involves generating a PKCS#12 certificate from
the separate `client.crt` and `client.key`, then importing that in the
web browser's certificate store.\
You may find more detailed instructions in
[Certificate Based Authentication](../tutorials/setup-operations-center-ui.md#certificate-based-authentication).

Alternatively, the UI can be accessed with OIDC login if configured on the
Operations Center service.

Once this is done, you can access the UI at `https://192.0.2.100:8443`
