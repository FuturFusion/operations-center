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
* `OPERATIONS_CENTER_E2E_TEST_NO_CLEANUP`: Disable cleanup of resources after tests, WARNING: this might cause errors, only use with single test cases (default: "false")
* `OPERATIONS_CENTER_E2E_TEST_NO_CLEANUP_ON_ERROR`: Disable cleanup of resources after failed tests, WARNING: this might cause errors, only use with single test cases or with `-failfast` flag of `go test` (default: "false")
* `OPERATIONS_CENTER_E2E_TEST_HOST_ADDRESS`: Address of the end 2 end test host, at which the services served in-process by the tests (the Redfish proxy and the fake OIDC provider) are reachable from the Operations Center VM. If not set, the address is derived from the network the Operations Center VM is attached to (default: "")
* `OPERATIONS_CENTER_E2E_TEST_BMC_PROXY_ADDRESS`: Same as `OPERATIONS_CENTER_E2E_TEST_HOST_ADDRESS`, but only for the in-process Redfish proxy used by the BMC tests. Takes precedence over `OPERATIONS_CENTER_E2E_TEST_HOST_ADDRESS` (default: "")

## Setup

A simple way to run the end to end tests as a developer is to create a VM using Incus
and run the tests inside the VM.

Make sure, that there is sufficient disk space available on the storage pool.
Since the end to end tests use snapshots, it is recommended to use a ZFS storage pool.

```shell
incus storage create zfs local-zfs
incus create images:debian/13 e2e --vm -c limits.cpu=8 -c limits.memory=24GiB
incus config device override e2e root size=50GiB io.cache=unsafe
incus storage volume create local-zfs zstorage --type=block size=150GiB
incus storage volume attach local-zfs zstorage e2e
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

#### Stage 1: Install ZFS and DKMS modules

```shell
apt update
apt install -y mokutil dkms
dkms autoinstall
DEBIAN_FRONTEND=noninteractive apt install -y linux-headers-amd64 zfs-dkms zfsutils-linux
mokutil --import /var/lib/dkms/mok.pub

# Halt the VM to complete the DKMS module installation.
shutdown -h now
```

Start the VM again:

```shell
incus start e2e --console
```

Enroll the MOK key.

#### Stage 2: Install required packages and get Operations Center repository

```shell
apt install -y curl jq golang git make build-essential systemd-timesyncd unzip bsdextrautils
systemctl enable systemd-timesyncd
systemctl restart systemd-timesyncd

# Install Incus
curl https://pkgs.zabbly.com/get/incus-stable | sudo sh

# Install OpenTofu
curl --proto '=https' --tlsv1.2 -fsSL https://get.opentofu.org/install-opentofu.sh -o install-opentofu.sh
chmod +x install-opentofu.sh
./install-opentofu.sh --install-method deb
rm -f install-opentofu.sh

# Add Docker's official GPG key:
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc

# Add the repository to Apt sources:
tee /etc/apt/sources.list.d/docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/debian
Suites: $(. /etc/os-release && echo "$VERSION_CODENAME")
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.asc
EOF
apt update
apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
```

Initialize Incus with ZFS storage backend using the ZFS block device:

```shell
incus admin init --auto --storage-backend=zfs --storage-create-device=/dev/disk/by-id/$(ls -1 /dev/disk/by-id | grep zstorage)
# Disable ZFS sync for better performance in ephemeral environments,
# since the tests are not concerned with data integrity in case of power loss.
#zfs set sync=disabled default
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

or a specific test:

```shell
make e2e-test GO_TEST_RUN=TestE2E_WithToken_CreateCluster
```

to show all available test cases:

```shell
make e2e-test-list
```

## Cleanup

```shell
make clean-e2e-test
```

## Development

### Shell commands

The test helpers (`run`, `mustRun` and friends) execute their commands with
`bash -o pipefail`, so a command, which fails anywhere in a pipeline, fails the
whole command.

### Idempotent tests

The existing end to end tests are designed to be run individually as well as in
a sequence. This means, that each test case should clean up any resources it
creates, so that the next test case can run without interference.

This is achieved by using `t.Cleanup` to register cleanup functions that are
executed after the test case finishes, regardless of whether it passes or fails.

The cleanup functions should be designed to not fail if the resource they are
trying to clean up does not exist.

In most cases, the cleanup functions should be registered before the actual
resource is created, to ensure that they are executed even if the resource
creation is only partially successful.

Examples:

* `t.Cleanup(clusterCleanup(t))`, cleans up any cluster created during the test
  case.
* `t.Cleanup(cleanupTokenSeed(t, token))`, cleans up the token seed created
  during the test case.
* `t.Cleanup(serverBMCConfigCleanup(t, tmpDir, name))`, resets the BMC config of
  a server, so that Operations Center stops talking to a BMC, which is about to
  go away.
* `t.Cleanup(ocIncusImageSourceCleanup(t, name))`, removes an Incus image
  source, which also removes all the images provided by that source.
* `t.Cleanup(systemSecurityOIDCCleanup(t, tmpDir))`, resets the OIDC part of the
  security config, so that Operations Center stops pointing at an OIDC issuer,
  which is about to go away.

For debug purposes, the cleanup can be disabled by setting the environment
variable `OPERATIONS_CENTER_E2E_TEST_NO_CLEANUP` or
`OPERATIONS_CENTER_E2E_TEST_NO_CLEANUP_ON_ERROR` to a truthy value.

### Fake BMC

The BMC support of Operations Center is tested against
[incus-redfish-proxy](https://github.com/FuturFusion/incus-redfish-proxy), which
exposes an Incus instance through a subset of the Redfish API. The proxy is
served in-process by the test on an ephemeral port of the end 2 end test host
(see `startRedfishProxy`), so no additional service needs to be installed.

The proxy presents a self signed certificate, which Operations Center pins
during the connection test performed by `provisioning server edit`.

The Operations Center VM reaches the proxy at the address of the end 2 end test
host on the network the VM is attached to. If this address can not be derived
automatically, set `OPERATIONS_CENTER_E2E_TEST_BMC_PROXY_ADDRESS`.

### Incus images

`TestE2E_WithToken_OCImagesRemoteLaunchInstance` covers the Incus image and the
Incus image source handling. It uploads images manually, syncs an image from
`https://images.linuxcontainers.org` through an image source, asserts the public
simplestreams endpoints of Operations Center and launches a container from a
manually uploaded and from a source synced image.

The filter expression of the image source is pinned to a single image and a
single version of that image, which is discovered from the image server at the
beginning of the test. This keeps the amount of downloaded data small and makes
the assertions on the synced images exact, even though the content of the image
server changes over time.

The Incus CLI caches the simplestreams responses of a remote for an hour, so the
test populates Operations Center completely before it registers Operations
Center as image remote on the server. Everything asserted after that point goes
through the public endpoints directly instead of through the server.

### Fake OIDC provider

The OIDC support of Operations Center is tested against a fake OIDC provider,
which is built on the storage of
[mini-oidc](https://github.com/lxc/incus/tree/main/test/mini-oidc), the same
building block the unit tests of the OIDC client use. The provider is served
in-process by the test on an ephemeral port of the end 2 end test host (see
`startOIDCProvider`), so no additional service needs to be installed.

Device authorizations are approved automatically, so no browser is involved.
The provider counts the device authorizations, which allows to verify, that a
refresh of the access token does not trigger a new interactive login.

The test uses an isolated config directory for the `operations-center` CLI
(`OPERATIONS_CENTER_CONF`). This way the CLI presents a fresh client
certificate, which is not trusted by Operations Center, so authentication can
only succeed through OIDC, and the remotes used by the other tests stay
untouched.
