# Server

Servers are the managed IncusOS instances that are registered with Operations
Center. Servers are deployed using a pre-seeded IncusOS installation image.
After installation, they automatically self-register themselves with Operations
Center using a [token](token.md).

Registered servers self-update their `connection_url` with Operations Center
periodically.

Operations Center on the other hand is periodically testing the connectivity
(by default every 5 Minutes) as well as updating the servers resources:

* Type
* Hardware data
* Operation system data
* Version information
* Certificate

Once one or many servers are clustered together to form a [cluster](cluster.md),
Operations Center will keep track of their [inventory](inventory.md).

## Automated Deployment

A pre-registered server with a BMC can be installed unattended:

```shell
operations-center provisioning server deploy <name> <token-uuid> <seed>
```

Operations Center then configures the BIOS of the server from the BIOS profiles
matching it, enrolls the secure boot certificates of IncusOS, attaches the
installation media generated from the token seed and boots it, and watches the
server until it has registered itself. The progress is followed with
`operations-center provisioning server deploy-status <name>` and a running
deployment is stopped with
`operations-center provisioning server deploy-cancel <name>`.

A BIOS attribute and a key database, that are correct already, are left alone,
so a server, that is deployed a second time, is neither power cycled nor has its
UEFI keys rewritten for nothing.

Not every BMC allows the UEFI key databases to be modified through its Redfish
API. Add `--skip-secure-boot-certificates` for such a server, in which case the
secure boot certificates of IncusOS have to be enrolled manually before the
deployment is triggered, everything else being unchanged.

While a deployment is running, the server is in status `deploying` with one of
the following status details:

| Server Status  | Server Status Detail           | Meaning                                                           |
| ---            | ---                            | ---                                                               |
| `deploying`    | `preparing`                    | Collecting BMC data, checking BIOS and powering the server off    |
| `deploying`    | `configuring BIOS`             | Applying and verifying the BIOS attributes, enrolling secure boot |
| `deploying`    | `attaching installation media` | Ejecting stale media and attaching the installation media         |
| `deploying`    | `installing`                   | The server is running the first stage of the installation         |
| `deploying`    | `finalizing`                   | Ejecting the media and waiting for the registration               |
| `deploying`    | `canceling`                    | Ejecting the media and powering the server off after a cancel     |
| `unregistered` | `deployment failed`            | The deployment failed, nothing has been cleaned up                |
| `unregistered` | `deployment canceled`          | The deployment has been canceled and cleaned up                   |

A successful deployment ends with the server registering itself, so it continues
its life in status `pending` and eventually graduating to `ready`. A failed
deployment is deliberately not cleaned up: the installation media stays attached
and the power state is left as it is, so the server can be inspected through the
BMC console.

More detailed information about the deployment can be found in
[/development/server-deployment].

## Network Configuration

Operations Center allows to update the network configuration of registered
servers. This is in particular useful for managing servers with multiple network
interfaces. See [IncusOS Network Configuration](https://linuxcontainers.org/incus-os/docs/main/reference/system/network/#configuration-options)
for more details.

## Update Operating System

Operations Center reports if updates are available, reboots are required or
if a server is currently in maintenance mode. Based on this information,
administrators can decide to trigger an update, evacuate workloads or reboot the
server.

| Server Status | Server Status Detail | Needs Update | Needs Reboot | Incus Cluster? | In Maintenance            | Aggregated Update State         | Recommended Action |
| ---           | ---                  | ---          | ---          | ---            | ---                       | ---                             | ---                |
| Ready         | -                    | false        | false        | -              | Not In Maintenance        | up to date                      | -                  |
| Ready         | -                    | true         | -            | -              | -                         | update pending                  | update             |
| Ready         | Updating             | -            | -            | -              | -                         | updating                        | -                  |
| Ready         | -                    | false        | true         | true           | Not In Maintenance        | evacuation pending              | evacuate           |
| Ready         | -                    | false        | -            | true           | In Maintenance Evacuating | evacuating                      | -                  |
| Ready         | -                    | false        | true         | true           | In Maintenance Evacuated  | in maintenance, reboot pending  | reboot             |
| Offline       | Rebooting            | -            | -            | true           | In Maintenance Evacuated  | in maintenance, rebooting       | -                  |
| Ready         | -                    | false        | -            | true           | In Maintenance Evacuated  | in maintenance, restore pending | restore            |
| Ready         | -                    | false        | -            | true           | In Maintenance Restoring  | restoring                       | -                  |
| Ready         | -                    | false        | true         | false          | Not In Maintenance        | reboot pending                  | reboot             |
| Offline       | Rebooting            | -            | false        | false          | Not In Maintenance        | rebooting                       | -                  |

Columns with `-` indicate that the value can be either `true` or `false` without
affecting the aggregated update state or recommended action.

For undefined states, the aggregated update state is `undefined` and the recommended action is `-` (none).

Actions "evacuate" and "restore" are only available, if the server has type "Incus".

More detailed information about the server status transitions can be found in [/development/server-status].
