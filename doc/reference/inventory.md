# Inventory

The goal of the inventory is to allow users to quickly find an object and then
access the correct Incus cluster to interact with it. It will also make it easy
to generate reports across the entire deployed Incus estate.

The global inventory keeps track of servers, clusters as well as as the majority
of the Incus objects available in the clusters managed by Operations Center.

Specifically, the following Incus objects are tracked:

* Image
* Instance
* Network
* Network ACL
* Network Address Set
* Network Forward
* Network Integration
* Network Load Balancer
* Network Peer
* Network Zone
* Profile
* Project
* Query
* Storage Bucket
* Storage Pool
* Storage Volume

For each object, the identity defining keys are tracked (e.g. `name`, `cluster`,
`server`, `project` and parent object).
Furthermore, the inventory contains the current state of these objects.

The inventory is updated both by processing life cycle events and periodic
full scans (in order to compensate for potential drift).

## Querying the Inventory

When querying the inventory, [expr-lang filters](filters.md#filtering-results-from-inventory)
maybe used for more sophisticated selection of results, in particular for
filtering based on object properties.
