# Cluster Tests

The cluster tests run an Operations Center cluster of three members, each one
as a separate `operations-centerd` process on the local host. The handover
test runs a cluster of four members, so one of them is a stand-by member.

## Usage

`make cluster-test` builds the daemons and runs the tests:

```shell
make cluster-test
```

It builds the daemon with `make bld`, which needs `UI=0` on a machine without
node/yarn. `CLUSTER_TEST_RUN` selects the tests to run, e.g.
`make cluster-test CLUSTER_TEST_RUN=TestUpgradeFromSQLite`.

Run directly, the tests are skipped, unless the environment variable
`OPERATIONS_CENTER_CLUSTER_TEST` is set to a non-empty value:

```shell
make bld
OPERATIONS_CENTER_CLUSTER_TEST=1 go test ./cluster_tests/ -v -count 1
```

The members listen on a global unicast IPv4 address of the host, which is
detected automatically, using a separate port each. No root privileges are
required.

Other environment variables, which control the tests:

* `OPERATIONS_CENTER_CLUSTER_TEST_BINARY`: Daemon to run (default: `../bin/operations-centerd`)
* `OPERATIONS_CENTER_CLUSTER_TEST_ADDRESS`: IP address the members listen on (default: the first global unicast IPv4 address of an interface, which is up)
* `OPERATIONS_CENTER_CLUSTER_TEST_PORT`: Port of the first member, the others use the following ones (default: `17443`)
* `OPERATIONS_CENTER_CLUSTER_TEST_NO_CLEANUP`: Keep the directories of the members after the tests (default: unset)
* `OPERATIONS_CENTER_CLUSTER_TEST_LEGACY_BINARY`: Daemon of a release up to 0.11.0, the last one storing its data in SQLite, for the upgrade test (default: unset, the upgrade test is skipped)

The upgrade test starts the legacy daemon as a standalone server, fills it
through its API and starts the current daemon on its directory. `make
cluster-test` installs the legacy daemon of the latest tag into `bin/`, or of
the tag set with `CLUSTER_TEST_LEGACY_VERSION`, e.g.
`make cluster-test CLUSTER_TEST_LEGACY_VERSION=v0.10.1`.

The directories of the members are created below `TMPDIR`. Since the path of
the unix socket of a member is limited in length, a short `TMPDIR` like
`/var/tmp` might be required.
