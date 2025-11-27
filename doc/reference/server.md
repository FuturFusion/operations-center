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
* Certificate

Once one or many servers are clustered together to form a [cluster](cluster.md),
Operations Center will keep track of their [inventory](inventory.md).

## Network Configuration

Operations Center allows to update the network configuration of registered
servers. This is in particular useful for managing servers with multiple network
interfaces. See [IncusOS Network Configuration](https://linuxcontainers.org/incus-os/docs/main/reference/system/network/#configuration-options)
for more details.
