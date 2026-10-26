# Operations Center cluster

Operations Center itself can run as a cluster of several members, which share
one database. Every member serves the complete API. Changes are handled by the
cluster leader, other members forward them to it.

## Create a cluster

Enable clustering on the first member, then issue a token for every further
member and join it with the token:

```
operations-center cluster enable --server-name m1 --server-address 192.0.2.11:8443
operations-center cluster tokens issue m2
operations-center cluster join <token> --server-name m2 --server-address 192.0.2.12:8443
```

The server address is used for the communication within the cluster and can
not be changed later. At least three members are needed, so the cluster stays
available while one member is down.

Every member serves the API on its server address. A member only needs
`network.rest_server_address`, if it should serve the API on another address
as well.

## Load balancer

Users and the managed servers reach the cluster through the address configured
as `network.address`, usually a load balancer forwarding to any of the members.
Enabling clustering sets it to the server address of the first member, unless
it is already set. It can be changed, but not removed, while clustered.

The load balancer has to pass TLS through, since the members present the
cluster certificate and the managed servers pin it.

## Configuration

Most of the configuration is shared by all members: the address of Operations
Center (`address` of `/1.0/system/network`), the server registration scriptlet
of `/1.0/system/settings`, and all of `/1.0/system/security`,
`/1.0/system/updates` and `/1.0/system/cluster`. The rest is local to a member:
the addresses it listens on (`rest_server_address` and `cluster_address`) and
its logging, metrics and pprof settings. The API presents both parts together,
so a client configures a member the same way it configures a standalone server.

The shared configuration is applied by all members at once. A change is refused
and reverted if an online member can not apply it. An offline member applies
the shared configuration when it starts again.

## Server records of the members

Every member is listed as server of type `operations-center`, named after its
`server-name` if clustering is enabled.

On IncusOS, the hosts of the members are managed through these records:

* A member is not powered off, rebooted or updated, while another member can
  not be reached, since the cluster could lose its quorum.
* IncusOS does not reboot the hosts on its own after an update, so the members
  are rebooted one at a time.
* All members follow `updates.server_default_channel`, since they have to run
  the same version.
* The network of a member can not be changed, since its address is part of the
  cluster.
