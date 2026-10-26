# Cluster Tests

The cluster tests run an Operations Center cluster of three members, each one
as a separate `operations-centerd` process on the local host.

## Usage

The tests are skipped, unless the environment variable
`OPERATIONS_CENTER_CLUSTER_TEST` is set to a non-empty value. They use the
daemon built by `make build`:

```shell
make build
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

The directories of the members are created below `TMPDIR`. Since the path of
the unix socket of a member is limited in length, a short `TMPDIR` like
`/var/tmp` might be required.
