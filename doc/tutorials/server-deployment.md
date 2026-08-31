# Automated Server Deployment

This guide walks through the unattended deployment of IncusOS onto a physical
server with Operations Center.

Operations Center drives the whole installation through the BMC of the server:
it configures the BIOS from the BIOS profiles matching the server, enrolls the
secure boot certificates of IncusOS, attaches the installation media generated
from a token seed and boots the server from it, and then watches the server
until it has registered itself.

A deployment usually takes quite some time, most of which is the server running
through its POST and through the first stage of the installation.

## Prerequisites

In order to execute this guide, you need:

* A running Operations Center instance, see
  [Deploy IncusOS Cluster](./deploy-incusos-cluster.md) for how to get one.
* A recent version of the `operations-center` CLI with a remote pointing at your
  Operations Center instance, get the suitable version for your system from
  [GitHub releases](https://github.com/FuturFusion/operations-center/releases).
* A supported server with a BMC, that speaks the Redfish API, and the
  credentials for it.
* Network connectivity in both directions: Operations Center has to reach the
  Redfish API of the BMC, and the BMC has to reach Operations Center, since it
  streams the installation media from there.
* An empty target disk in the server. The installer refuses to overwrite an
  existing IncusOS installation, see [Retry a Deployment](#retry-a-deployment).

### Configure the Address of Operations Center

The installation media URL handed to the BMC is built from the `address`
setting of Operations Center, so it has to be set to an address, that the BMC
can actually reach. A deployment is rejected right away, if it is not
configured.

Show the current network configuration:

```shell
operations-center system network show
```

If `address` is empty or not reachable for the BMC, set it:

```shell
operations-center system network edit
```

```yaml
address: https://operations-center.some.tld:8443
rest_server_address: :8443
```

```{note}
Keep the address short. Some BMCs cut the image URL off after 255 characters,
and BMC firmware is known to parse the combination of an IPv6 address with a
non-default port incorrectly.
```

### Wait for Updates

The installation media is generated from the updates cached by Operations
Center, so at least one update has to be in state `ready` before a deployment
can be started:

```shell
operations-center provisioning update list
```

## Check the Target System

IncusOS relies on a TPM 2.0 and on secure boot for its security features. At
least one of the two has to be available for the installation to work, and the
answers to the following questions determine, what the token seed created below
has to contain.

**Does the target system have a TPM 2.0?**

* If yes, nothing needs to be done.
* If no, set `install.security.missing_tpm: true` in the token seed. This only
  works, if secure boot is available, see the next question.

**Does the target system support secure boot?**

* If yes, does the BMC allow the secure boot certificates to be configured
  through its Redfish API?
   * If yes, nothing needs to be done, the deployment enrolls the secure boot
     certificates of IncusOS automatically.
   * If no, enroll the certificates manually, see
     [Enroll the Secure Boot Certificates Manually](#enroll-the-secure-boot-certificates-manually),
     and pass `--skip-secure-boot-certificates` to the deployment later on.
* If no, set `install.security.missing_secure_boot: true` in the token seed.
  This only works, if a TPM 2.0 is available, see the previous question.

```{note}
Remember, at least one of TPM 2.0 and secure boot needs to be available for the
installation to work. Both `missing_tpm` and `missing_secure_boot` reduce the
security of the system compared to properly configured secure boot and a TPM,
so only use them when you have to.
```

Whether the BMC reports a TPM can also be read from Operations Center itself
once the server is pre-registered, see
[Check what Operations Center Sees](#check-what-operations-center-sees).

## Create a Token

Create a new provisioning [token](../reference/token.md), which the server uses
to register itself with Operations Center after the installation:

```shell
operations-center provisioning token add --description "Server deployment tutorial" --uses 5
```

List the tokens to get the `UUID` of the token just created and note it down:

```shell
operations-center provisioning token list
```

```{note}
Give the token enough uses for the retries you might need. Every deployment
consumes one use of the token when the server registers itself. The token also
has to stay valid for the whole deployment, which takes up to two hours.
```

## Create the Token Seed

The [token seed](../reference/token.md#token-seed) describes what is installed
on the server and how. Create a file `deploy-seed.yaml` with the following
content and adjust it to your target system:

```yaml
applications:
  version: "1"
  applications:
    - name: incus
install:
  version: "1"
  force_install: false
  force_reboot: true
  security:
    missing_tpm: false
    missing_secure_boot: false
  target: null
network:
  version: "1"
  dns:
    domain: some.tld
    hostname: myhost
    nameservers:
      - 1.0.0.1
      - 1.1.1.1
      - 2606:4700:4700::1001
      - 2606:4700:4700::1111
  time:
    timezone: UTC
```

The important aspects are:

* `applications`: the application to be installed, most likely `incus`.
* `install.target`: only needed, if the system has more than one installation
  target (disk) to choose from. Without it, the installer expects a single disk
  to be present. The target can be selected by `bus`, `id`, `min_size`,
  `max_size` or `sort_order`, see
  [IncusOS Installation Seed](https://linuxcontainers.org/incus-os/docs/main/reference/seed/).
* `install.security`: set `missing_tpm` or `missing_secure_boot` according to
  [Check the Target System](#check-the-target-system).
* `network`: only needed, if the network configuration is not obtained via DHCP.

Optional, but recommended:

* `install.force_reboot: true` lets the server reboot on its own once the first
  stage of the installation is done, which speeds the deployment up. It does not
  work on all systems though. Without it, the deployment relies on the read
  progress of the installation media alone and `--force` has to be passed to
  `server deploy`.

Now add the seed to the token. Replace `<token-uuid>` with the `UUID` noted down
above:

```shell
operations-center provisioning token seed add <token-uuid> server-deployment-tutorial deploy-seed.yaml --public --description "Server deployment tutorial"
```

```{note}
The `--public` flag is required for the automated deployment. The BMC fetches
the installation media without authentication, so a deployment referencing a
token seed, that is not public, is rejected.
```

Verify the seed:

```shell
operations-center provisioning token seed list <token-uuid>
operations-center provisioning token seed show <token-uuid> server-deployment-tutorial
```

## Pre-register the Server

A server, that is to be deployed, has to be known to Operations Center together
with the connection details of its BMC. Replace the placeholders with the name
you want to give the server and the address and credentials of its BMC:

```shell
operations-center provisioning server pre-register <server-name> \
  --bmc-api-type redfish-v1-generic \
  --bmc-auto-pin-certificate \
  --bmc-endpoint https://<bmc-address> \
  --bmc-username <bmc-user-name> \
  --bmc-password <bmc-password> \
  --description "Some description of the server"
```

`--bmc-auto-pin-certificate` accepts and pins the certificate presented by the
BMC on first contact. If you have the server certificate of the BMC at hand,
provide it with `--bmc-certificate-file <filename>` instead. The two flags are
mutually exclusive.

The pre-registered server shows up in status `unregistered`:

```shell
operations-center provisioning server list
```

## Check what Operations Center Sees

Pre-registering the server makes Operations Center collect the data of its BMC,
which is worth a look before the deployment is started.

Show the BMC data of the server:

```shell
operations-center provisioning server show <server-name> --bmc-data
```

Interesting fields are:

* `server_manufacturer` and `server_model`: what the BIOS profiles are matched
  against.
* `server_has_tpm`: whether the BMC reports a TPM, which answers the first of
  the questions in [Check the Target System](#check-the-target-system).
* `virtual_media`: the virtual media devices the installation media can be
  attached to, keyed by `<service>:<id>`, e.g. `system:1`.

Show the BIOS attributes, that would be applied to the server before IncusOS is
installed on it:

```shell
operations-center provisioning server bios-profile <server-name>
```

```text
Profiles: dell, dell-with-tpm
Attributes:
  SecureBoot: Enabled
  SecureBootMode: UserMode
  SecureBootPolicy: Custom
  TpmSecurity: On
Deferred attributes:
  Tpm2Algorithm: SHA256
```

The attributes are accumulated from all the BIOS profiles matching the data
reported by the BMC, processed by priority ascending. The catalog shipped with
Operations Center currently covers Dell, Lenovo and HPE systems. A deployment of
a server, that no profile matches, is rejected.

Add `--validate` to check the resolved attributes against the BIOS attribute
registry of the BMC, which catches an attribute name or value the firmware of
this particular system does not know:

```shell
operations-center provisioning server bios-profile <server-name> --validate
```

## Enroll the Secure Boot Certificates Manually

Not every BMC allows the UEFI key databases to be modified through its Redfish
API. For such a system, the secure boot certificates of IncusOS have to be
enrolled through the BIOS setup **before** the deployment is started, and
`--skip-secure-boot-certificates` has to be passed to `server deploy`.

```{note}
The order matters. With secure boot enabled and the certificates of IncusOS
missing from the key databases, the firmware refuses to boot the installation
media at all, so the deployment does not get anywhere. The certificates have to
be in place before the server is booted from the installation media.
```

The certificates are shipped as `.der` files in the `keys` folder of the
installation media, so fetch the image of the token seed to get at them:

```shell
operations-center provisioning token seed get-image <token-uuid> server-deployment-tutorial ~/Downloads/IncusOS.iso --type iso
```

Then enroll them through the BIOS setup of the server. The exact mechanism
differs widely between vendors, but the sequence is always the same:

1. Enable secure boot.
1. Wipe the KEK and enroll the IncusOS KEK key (`secureboot-KEK-R1.der`).
1. Wipe the DB and enroll the IncusOS DB key(s).
1. Save the settings and power the server off.

The KEK has to be enrolled before the DB keys.

```{note}
Do not blindly wipe everything from the DB. The NVMe and storage controllers of
some servers rely on the Microsoft UEFI CA or the vendors own certificates to
boot, so on such a system those keys have to be retained.
```

See [Installing on a physical machine](https://linuxcontainers.org/incus-os/docs/main/getting-started/installation/physical/)
in the IncusOS documentation for the vendor specific details.

The BIOS attributes are still applied by the deployment in this case, only the
enrollment of the certificates is skipped.

## Start the Deployment

Everything is in place now, so the deployment can be started. Replace
`<server-name>` with the name used for the pre-registration and `<token-uuid>`
with the `UUID` of the token:

```shell
operations-center provisioning server deploy <server-name> <token-uuid> server-deployment-tutorial --wait
```

The relevant flags are:

* `--wait`: watch the progress and wait for the deployment to complete. Without
  it, the command returns as soon as the deployment has been accepted.
* `--skip-secure-boot-certificates`: pass this, if the system does not support
  the management of the secure boot certificates through its Redfish API. The
  certificates have to be enrolled manually before the deployment is triggered
  in this case, see
  [Enroll the Secure Boot Certificates Manually](#enroll-the-secure-boot-certificates-manually).
* `--virtual-media-id <virtual-media-id>`: only needed, if the automatically
  selected virtual media device is not the wanted one. Without the flag, the
  first device advertising CD or DVD support is used, the devices offered by the
  system taking precedence over the ones offered by the manager.
* `--type raw`: by default, the ISO image is used for the installation. Some
  virtual media implementations do not support the ISO, in which case the raw
  image has to be used. Unfortunately, this is only detected mid deployment.
* `--force`: needed, if the token seed does not set `force_reboot`. The
  deployment then relies on the read progress of the installation media alone to
  detect the end of the first installation stage.

Everything, that can be checked up front, is checked when the deployment is
requested, so an impossible deployment fails right here rather than halfway
through.

## Watch the Progress

With `--wait`, the CLI prints every state the deployment enters together with
the time it entered it:

```text
2026-09-02T09:12:01Z refresh-bmc-data
2026-09-02T09:12:11Z check-bios
2026-09-02T09:12:21Z power-off-bios
2026-09-02T09:12:31Z wait-power-off-bios
2026-09-02T09:13:41Z apply-bios
...
2026-09-02T09:20:33Z verify-bios
  retries: 1
2026-09-02T09:21:47Z wait-install
2026-09-02T09:48:12Z detach-media
2026-09-02T09:49:02Z wait-reboot
2026-09-02T09:52:38Z wait-registration
2026-09-02T09:58:11Z cleanup
2026-09-02T09:58:21Z completed
```

A step, that had to be repeated — because the BMC asked for the operation to be
retried, or because a wait fell back to the trigger it belongs to — reports how
often underneath the state. What went wrong is not reported here, since the
deployment recovered from it. It is reported when it ends the deployment: the
command then exits with the state the deployment failed in and the error, that
got it there.

The same information, plus what the deployment is applying, is available at any
time from another terminal:

```shell
operations-center provisioning server deploy-status <server-name>
```

```text
State: wait-install
Started at: 2026-09-02T09:12:01Z
State entered at: 2026-09-02T09:21:47Z
Token: 4b2d1c9e-6f3a-4c58-9a7d-2e5b8c1f0a34
Seed: deploy
Virtual media: system:1
Force reboot: true
Skip secure boot certificates: false
Media URL: https://operations-center.some.tld:8443/1.0/provisioning/tokens/...
Media read: 423.01MB
BIOS profiles: dell, dell-with-tpm
BIOS attributes:
  SecureBoot: Enabled
  SecureBootMode: UserMode
  SecureBootPolicy: Custom
  TpmSecurity: On
BIOS deferred attributes:
  Tpm2Algorithm: SHA256
History:
  2026-09-02T09:12:01Z refresh-bmc-data
  2026-09-02T09:12:11Z check-bios
  ...
```

Add `--format yaml` or `--format json` to get the full status for further
processing.

While the deployment runs, the server is in status `deploying` with a status
detail telling what is going on:

```shell
operations-center provisioning server list
```

The status details and what they mean are documented in
[Automated Deployment](../reference/server.md#automated-deployment).

A running deployment can be stopped at any time:

```shell
operations-center provisioning server deploy-cancel <server-name>
```

This ejects the installation media and powers the server off. A deployment,
that failed on its own, is deliberately left untouched instead, so the server
can be inspected through the BMC console.

## After the Deployment

A successful deployment ends with the server registering itself, so it shows up
in status `pending` and eventually graduates to `ready`:

```shell
operations-center provisioning server list
```

The server name defaults to the host name of the machine, which, if not set via
the `network.dns.hostname` of the token seed, falls back to the machine ID. To
give it a more meaningful name:

```shell
operations-center provisioning server rename <current-server-name> <new-server-name>
```

From here on, the server is an ordinary [server](../reference/server.md) in
Operations Center and can be clustered, as described in the
[Deploy IncusOS Cluster](./deploy-incusos-cluster.md#cluster-the-servers)
tutorial.

## Retry a Deployment

The installer refuses to overwrite an existing installation, so a server, that
has already been installed once, needs its disks wiped before it can be deployed
again.

To start over with the same server:

```shell
operations-center provisioning server remove <server-name>
```

Then continue with [Pre-register the Server](#pre-register-the-server) again.

```{note}
Removing the server is only necessary, if it registered itself in the meantime.
A server, whose deployment failed, stays `unregistered` and can simply be
deployed again.
```

## Troubleshooting

* **The deployment is rejected right away.** The pre-flight checks report what
  is wrong: a missing address of Operations Center, a token that is exhausted or
  expired, a token seed that is not public or does not set `force_reboot`, a BMC
  reporting no virtual media device, or no BIOS profile matching the server.
* **The deployment fails in `wait-install`.** The server did not get through the
  first stage of the installation. The installation media stays attached and the
  power state is left as it is, so open the BMC console and look at what the
  installer reported. If the server fails to boot from the ISO, try
  with `--type raw`.
* **The deployment fails while applying the BIOS.** Check the resolved
  attributes against the attribute registry of the BMC with
  `operations-center provisioning server bios-profile <server-name> --validate`
  and have a look at the BIOS attributes the BMC reports with
  `operations-center provisioning server bmc bios-attributes list <server-name>`.
  If there is a mismatch, reach out such that the profiles in Operations Center
  can be extended.
* **The deployment fails in `wait-registration`.** The installation finished,
  but the server did not reach Operations Center. Check that the network
  configuration in the token seed is correct and that the server can reach the
  address of Operations Center.
* **Look at the BMC logs** for what the BMC itself has to say about the server:

  ```shell
  operations-center provisioning server bmc logs <server-name>
  operations-center provisioning server bmc log-entries <server-name> <log-source>
  ```

  The log source has the structure `<service>/<logService>`, e.g.
  `chassis/Logs`, and the available sources are listed by the `logs` command.

The internals of the automated deployment, including the state machine and the
timeouts bounding every step, are documented in
[Server Deployment](../development/server-deployment.md).
