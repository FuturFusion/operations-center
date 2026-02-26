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
