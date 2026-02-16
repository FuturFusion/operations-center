# Channel

Channels represent a logical grouping of [updates](update.md), which define
which updates should be made available to which [servers](server.md)
(IncusOS instance). Every update is part of at least one channel but it is
possible for an update to be part of multiple channels.

Channels in Operations Center are distinct from the upstream channels defined by
the update metadata, which are used to group updates on the source side.

Each managed server as well as each [cluster](cluster.md) is assigned to a
single channel. Operations Center will enforce configuration of the respective
channel on the managed servers, such that they only consider updates from the
configured channel.

Every update is assigned to the default channel upon retrieval by Operations
Center. It is possible to change the assignment of an update to different
channels.

Every server or cluster is assigned to a default channel upon registration or
creation in Operations Center. This assignment can be changed later on as well.

It is possible to change the default channel for updates and servers/clusters in
the Operations Center [updates system settings](settings.md#update-settings).
This will only affect new updates and servers/clusters, but not existing ones.
