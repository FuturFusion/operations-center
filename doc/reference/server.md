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

During normal update flow, the following states are passed:

| needs update | needs reboot | in maintenance | recommended action                             |
| :---         | :---         | :---           | :---                                           |
| false        | false        | false          | <none>                                         |
| true         | false        | false          | update                                         |
| false        | true         | false          | if `type == "incus"`: "evacuate" else "reboot" |
| false        | true         | true           | reboot                                         |
| false        | false        | true           | restore                                        |

The following states are also possible, but less likely to be encountered during normal operation:

| needs update | needs reboot | in maintenance | recommended action |
| :---         | :---         | :---           | :---               |
| true         | true         | false          | update             |
| true         | false        | true           | update             |
| true         | true         | true           | update             |

Actions "evacuate" and "restore" are only available, if the server has type "Incus".
