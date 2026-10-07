# BIOS Profiles

## Deferred attributes

`attributes` are applied in one go and picked up by the firmware on the next
reset. An attribute, that the firmware only accepts once another attribute is
actually in effect belongs into `deferred_attributes`, which the automated
deployment applies in a second pass, after the server has been rebooted and the
attributes of the first pass are in effect.

## Secure boot allow lists

The automated deployment wipes the `KEK`, `db` and `dbx` key databases of a
server and reinitializes them with the certificates of IncusOS. The `secure_boot`
section of a profile names the entries, that survive that wipe, keyed by the
lower case hex encoded SHA256 fingerprint of the certificate or by the signature
value:

* `true` keeps the entry.
* `false` removes it again, which only ever undoes a `true` of a profile resolved
  before, since nothing is allow listed out of the box.
* A null value drops the entry from the set accumulated by the profiles resolved
  before, so a profile with a higher priority can undo a decision rather than
  overrule it.

The profiles are the only source of the allow list, so a profile, that names
nothing, has the `db` of its servers wiped down to the certificates of IncusOS
and the option ROMs of the hardware stop being trusted. A profile for hardware,
that none of the profiles shipped with Operations Center match, therefore has to
name the Microsoft CAs itself, see [`_dummy.yaml.example`](_dummy.yaml.example).

A key database, that holds the certificates of IncusOS plus the allow listed
entries and nothing else, is left untouched entirely, which spares a server, that
is deployed a second time, having its UEFI keys deleted and rewritten.

The allow list also decides, what the secure boot enrollment media re-enrolls. A
deployment, that enrolls the certificates from an enrollment media rather than
through the Redfish API, starts from wiped key databases, so an allow listed
entry has to be enrolled again from actual certificate material.

## Deployment settings

The timings of the automated deployment suit most hardware. Where a
manufacturer, a model or a firmware version needs more time, the `deployment`
section of a profile overrides them for the servers the profile matches. A
setting, that is not set, leaves in place what the profiles with a lower priority
have set, and finally the default. Durations are Go duration strings, e.g. `90s`
or `1h30m`.

| Setting                         | Default     | Meaning                                                                                               |
| ---                             | ---         | ---                                                                                                   |
| `deployment_timeout`            | `2h`        | Bounds the deployment as a whole, the provisioning token has to stay valid for it                     |
| `step_timeout`                  | `5m`        | Bounds the waits for a power off, for the virtual media and for the cancellation                      |
| `bios_applied_timeout`          | `10m`       | Bounds the waits for the firmware to apply BIOS and secure boot changes                               |
| `secure_boot_enroll_timeout`    | `15m`       | Bounds the wait for the enrollment media to enroll the secure boot certificates                       |
| `install_timeout`               | `45m`       | Bounds the first stage of the installation                                                            |
| `reboot_timeout`                | `15m`       | Bounds the wait for the server to come back up after the first stage of the installation              |
| `registration_timeout`          | `30m`       | Bounds the wait for the server to register itself                                                     |
| `post_settle_delay`             | `1m`        | Time granted to the power on self test, before a wait trusts what the BMC reports                     |
| `power_off_settle_delay`        | `1m`        | Time the server has to be reported powered off, before the power off counts as settled                |
| `reboot_observation_window`     | `5m`        | Time the wait for the reboot looks for an actual reboot, before it settles for the power state        |
| `secure_boot_settle_duration`   | `5m`        | Time the server is left running after the certificates have been enrolled, if it does not reboot      |
| `install_min_duration`          | `5m`        | Time, that has to have passed, before the first stage of the installation could be done at all        |
| `install_reboot_fallback_delay` | `10m`       | Time, after which a reboot counts as the end of the first stage, where the BMC reports no progress    |
| `install_media_idle_period`     | `2m`        | Time without a read, after which the installation media counts as idle                                |
| `install_media_min_bytes_read`  | `524288000` | Bytes of the installation media, that have to have been read, before the idle period counts (500 MiB) |
| `step_retries`                  | `3`         | Retries granted to a single step, as well as fallbacks and reverts granted to a single wait           |
| `call_timeout`                  | `2m`        | Bounds the BMC operations of a single attempt of a step                                               |
| `attach_media_call_timeout`     | `20m`       | Bounds the BMC operations attaching a virtual media                                                   |
| `secure_boot_call_timeout`      | `15m`       | Bounds the BMC operations changing the secure boot key databases                                      |

A duration has to be between `1s` and `24h`, `step_retries` between 1 and 10 and
`install_media_min_bytes_read` at least 1. When a deployment is requested, the
resolved settings additionally have to fit together: what a wait holds out for
has to be shorter than its timeout, and no timeout, the call timeouts included,
may exceed `deployment_timeout`. Lowering `deployment_timeout` below a default,
e.g. the `45m` of `install_timeout`, therefore requires lowering that setting as
well.

A setting with an unknown name is rejected. The settings deviating from the
defaults for a server are shown by
`operations-center provisioning server bios-profile <name>`, the ones of a
deployment by `operations-center provisioning server deploy-status <name>`.

## Example

An example of a BIOS profile, showing all the fields available for matching and
for the values to apply, can be found in
[`_dummy.yaml.example`](_dummy.yaml.example).

## Development

Profiles under development do not have to be compiled into Operations Center
to be tried out. `operations-center provisioning server deploy` accepts files
in the format of this directory with `--bios-profiles <file>` and resolves the
BIOS configuration from them instead of the profiles in this directory, taking
their matches and their priorities into account the same way. Such a file is
checked more strictly: a field with an unknown name is rejected and the
certificates have to be keyed by their SHA256 fingerprint. A certificate, that
is not part of the certificate catalog yet, has to be provided with
`--secure-boot-certificate <file>` for the enrollment media.

## Tests

If new profiles are added, it is advised to also update
[`../profiles_test.go`](../profiles_test.go) with additional tests for the
manufacturers, models and hardware combinations covered by the new profiles, to
make sure the profiles provide the expected outcome.
