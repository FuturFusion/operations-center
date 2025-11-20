# End to End Tests

## Usage

In order to run the end to end tests, the environment variable `OPERATIONS_CENTER_E2E_TEST`
needs to be set to a truthy value when running `go test`.

```shell
export OPERATIONS_CENTER_E2E_TEST=1
go test ./e2e_tests/ -v -timeout 60m -count 1 | tee e2e_tests_$(date +%F-%H-%M-%S).out
```

Use `-count 1` to disable test caching, which is important since the tests have side effects
that would not be re-executed on a cached run.

Use `-timeout` to set a suitable timeout for the tests.

Use `-run` to run specific tests, e.g. `-run TestE2E/create_cluster`.

Other environment variables that can be set to control the tests:

* `OPERATIONS_CENTER_E2E_TEST_TMP_DIR`: Directory to use for temporary files. If not set, a temporary directory will be created and removed automatically. This is useful for developers, since artifacts (e.g. ISO files) are taken from the temporary directory if present, which speeds up the tests on subsequent runs.
* `OPERATIONS_CENTER_E2E_TEST_DISK_SIZE`: Disk size to use for the IncusOS instances (default: "50GiB")
* `OPERATIONS_CENTER_E2E_TEST_MEMORY_SIZE`: Memory size to use for the IncusOS instances (default: "4GiB")
* `OPERATIONS_CENTER_E2E_TEST_CPU_COUNT`: CPU count to use for the IncusOS instances (default: "2")
* `OPERATIONS_CENTER_E2E_TEST_CONCURRENT_SETUP`: Whether to setup the IncusOS instances concurrently (default: "true")
* `OPERATIONS_CENTER_E2E_TEST_TIMEOUT_STRETCH_FACTOR`: Factor to stretch timeouts by (default: "1.0")
* `OPERATIONS_CENTER_E2E_TEST_CPU_ARCH`: CPU architecture used (default: "amd64")
* `OPERATIONS_CENTER_E2E_TEST_DEBUG`: Enable debug output (default: "false")
* `OPERATIONS_CENTER_E2E_TEST_NO_CLEANUP`: Disable cleanup of resources after tests, WARNING: this might cause errors (default: "false")

## Setup

A simple way to run the end to end tests as a developer is to create a VM using Incus
and run the tests inside the VM.

Make sure, that there is sufficient disk space available on the storage pool.

```shell
incus create images:debian/13 e2e --vm -c limits.cpu=4 -c limits.memory=20GiB
incus config device override e2e root size=200GiB
incus start e2e
incus exec e2e -- bash
```

> **Note:**
> With more advanced setups, the performance of the tests, in particular the
> snapshot creation and restoration, can be improved significantly. If this is
> a concern, consider using ZFS for the storage volume.
>
> Be aware, that using ZFS inside the VM requires installation of the specific
> ZFS packages and DKMS modules.

### Install software inside the VM

```shell
apt update
apt install -y curl jq golang git make build-essential systemd-timesyncd unzip
curl https://pkgs.zabbly.com/get/incus-stable | sudo sh
curl --proto '=https' --tlsv1.2 -fsSL https://get.opentofu.org/install-opentofu.sh -o install-opentofu.sh
chmod +x install-opentofu.sh
./install-opentofu.sh --install-method deb
rm -f install-opentofu.sh
```

Get the source code and build the Operations Center binaries:

```shell
git clone https://github.com/FuturFusion/operations-center.git
cd operations-center
go get -v ./...
make build
```

### Run the tests

```shell
export OPERATIONS_CENTER_E2E_TEST=1
export OPERATIONS_CENTER_E2E_TEST_TMP_DIR=$HOME/tmp-e2e
go test ./e2e_tests/ -v -timeout 60m -count 1 | tee e2e_tests.out
```

or

```shell
make e2e-test
```

## Cleanup

```shell
make clean-e2e-test
```
