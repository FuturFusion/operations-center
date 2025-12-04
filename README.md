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

# Documentation

Some more detailed information about various aspects of Operations Center can be
found at [`https://docs.futurfusion.io/operations-center`](https://docs.futurfusion.io/operations-center).

# Bug reports

You can file bug reports and feature requests at:
[`https://github.com/futurfusion/operations-center/issues/new`](https://github.com/futurfusion/operations-center/issues/new)

# Development

This repository includes all the sources used to build Operations Center.

Beside the regular CI pipeline, which is executed for every pull request, a
daily end-to-end test is also run, exercising the most involved API endpoints
(e.g. full cluster creation) and running tests that would be impractical (too
slow) to run for every pull running tests.

[![Daily API tests](https://github.com/futurfusion/operations-center/actions/workflows/daily.yml/badge.svg)](https://github.com/futurfusion/operations-center/actions/workflows/daily.yml)

# Contributing

This repository is released under the terms of the Apache 2.0 license.

Fixes and new features are greatly appreciated. Make sure to read our
[contributing guidelines](https://docs.futurfusion.io/operations-center/main/contributing)
first!
