# Operations Center

Operations Center is your central overview of your Incus deployment.

It fulfills four main roles:

* Acts as the registration point for all servers running the IncusOS
* Handles update tracking and rollout across the entire cloud
* Provisions new Incus clusters
* Keeps track of resources across all clusters

Most cloud users will interact with the Operations Center to track their own
cloud usage and see what workloads they have on what cluster.

Operations Center runs as a service, exposing a REST API with both a
multi-platform command line tool as well as a web interface as clients.

Through that, the user can create deployment tokens and pre-seeded IncusOS
images (ISO and raw) for easy bootstrapping of new IncusOS servers, which then
can be clustered together. Operations Center then takes care of providing
updates to the IncusOS servers in the clusters it manages.
Furthermore, Operations Center provides an inventory view of all resources
across all clusters.
