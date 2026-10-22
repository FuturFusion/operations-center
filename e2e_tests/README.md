# End to End Tests

## Usage

In order to run the end to end tests, the environment variable `OPERATIONS_CENTER_E2E_TEST`
needs to be set to a truthy value when running `go test`.

```shell
export OPERATIONS_CENTER_E2E_TEST=1
go test ./e2e_tests/ -v -timeout 180m -count 1 | tee e2e_tests_$(date +%F-%H-%M-%S).out
```

Use `-count 1` to disable test caching, which is important since the tests have side effects
that would not be re-executed on a cached run.

Use `-timeout` to set a suitable timeout for the tests.

Use `-run` to run specific tests, e.g.
`-run '^TestE2E_FromManualUpload_CreateCluster$'`.
Use `-run '^TestE2E_OperationsCenter$/^oidc_authentication$'` to run a single
scenario of a suite (see [Suites and scenarios](#suites-and-scenarios)).

Other environment variables that can be set to control the tests:

* `OPERATIONS_CENTER_E2E_TEST_TMP_DIR`: Directory to use for temporary files. If not set, a temporary directory is created below the home directory of the current user. It is *not* removed automatically. Setting this explicitly is recommended for developers, since artifacts (e.g. ISO files) are taken from the temporary directory if present, which speeds up the tests on subsequent runs. A cached IncusOS image is validated before it is reused and discarded, if it is not usable, so a failed download does not poison the directory.
* `OPERATIONS_CENTER_E2E_TEST_DISK_SIZE`: Disk size to use for the IncusOS instances (default: "50GiB")
* `OPERATIONS_CENTER_E2E_TEST_MEMORY_SIZE`: Memory size to use for the IncusOS instances (default: "4GiB")
* `OPERATIONS_CENTER_E2E_TEST_CPU_COUNT`: CPU count to use for the IncusOS instances (default: "2")
* `OPERATIONS_CENTER_E2E_TEST_CONCURRENT_SETUP`: Whether to setup the IncusOS instances concurrently (default: "true")
* `OPERATIONS_CENTER_E2E_TEST_TIMEOUT_STRETCH_FACTOR`: Factor to stretch timeouts by (default: "1.0"). This also stretches the deployment timeouts in the BIOS profile of `TestE2E_WithTokenSeed_DeployServer`. It does not change the `-timeout` flag of `go test`. Raise this flag by the same factor.
* `OPERATIONS_CENTER_E2E_TEST_TIMEOUT`: Timeout for a single test case or a single scenario of a suite, subject to `OPERATIONS_CENTER_E2E_TEST_TIMEOUT_STRETCH_FACTOR` (default: "60m"). This is not the same as the `-timeout` flag of `go test`, which is a budget shared by all the test cases of the suite. Exceeding the timeout of a single test case fails this test case, which allows the cleanup and the collection of the debug output to take place, while exceeding the `go test` timeout panics the whole test binary.
* `OPERATIONS_CENTER_E2E_TEST_CPU_ARCH`: CPU architecture used (default: "amd64")
* `OPERATIONS_CENTER_E2E_TEST_DEBUG`: Enable debug output (default: "false")
* `OPERATIONS_CENTER_E2E_TEST_NO_CLEANUP`: Disable cleanup of resources after tests, WARNING: this might cause errors, only use with single test cases (default: "false")
* `OPERATIONS_CENTER_E2E_TEST_NO_CLEANUP_ON_ERROR`: Disable cleanup of resources after failed tests, WARNING: this might cause errors, only use with single test cases or with `-failfast` flag of `go test` (default: "false")
* `OPERATIONS_CENTER_E2E_TEST_LIST`: Only list the test cases and the scenarios of the suites, do not run them. `make e2e-test-list` uses this (default: "false")
* `OPERATIONS_CENTER_E2E_TEST_HOST_ADDRESS`: Address of the end 2 end test host. The Operations Center VM reaches the services of the tests at this address (the Redfish proxy, the fake OIDC provider, and the OpenFGA server). If not set, the address is derived from the network the Operations Center VM is attached to (default: "")
* `OPERATIONS_CENTER_E2E_TEST_BMC_PROXY_ADDRESS`: Same as `OPERATIONS_CENTER_E2E_TEST_HOST_ADDRESS`, but only for the in-process Redfish proxy used by the BMC tests. Takes precedence over `OPERATIONS_CENTER_E2E_TEST_HOST_ADDRESS` (default: "")

## Test structure

The end to end tests consist of suites and of single tests. A suite runs
several scenarios in a fixed order. The scenarios of a suite share a pool of
IncusOS servers, so the servers are only installed once per suite in the best
case. See [Suites and scenarios](#suites-and-scenarios) for the details.

All the tests share one Operations Center VM. The tests run in the following
order:

| Top level test                           | Server pool                                  | Content                                                                    |
|------------------------------------------|----------------------------------------------|----------------------------------------------------------------------------|
| `TestE2E_OperationsCenter`               | none                                         | Suite of the scenarios, which need no IncusOS server                       |
| `TestE2E_StableServers`                  | IncusOS01 to IncusOS04 on the stable channel | Suite of the scenarios with servers and clusters                           |
| `TestE2E_UpdateChannel`                  | IncusOS01 to IncusOS04 on the prod channel   | Suite of the update scenarios                                              |
| `TestE2E_WithTokenSeed_DeployServer`     | none                                         | Single test, deploys IncusOS01 through its BMC                             |
| `TestE2E_FromManualUpload_CreateCluster` | none                                         | Single test, creates a single node cluster from a manually uploaded update |

### `TestE2E_OperationsCenter`

| Scenario                               | Content                                                                                                      |
|----------------------------------------|--------------------------------------------------------------------------------------------------------------|
| `basic_interactions`                   | CLI commands for admin, query, system, debug, metrics, backup and restore, token seeds and cluster templates |
| `provisioning_updates_channels_tokens` | Updates, channels and tokens, see [Updates, channels and tokens](#updates-channels-and-tokens)               |
| `oidc_authentication`                  | Authentication through OIDC, see [Fake OIDC provider](#fake-oidc-provider)                                   |
| `openfga_authorization`                | Authorization through OpenFGA, see [OpenFGA server](#openfga-server)                                         |
| `updates_cleanup_and_refresh`          | Removes all the updates and refreshes them. Runs last.                                                       |

### `TestE2E_StableServers`

The servers register with a token of the stable channel. The registration
scriptlet sets their description and their properties. IncusOS01 is installed
from the ISO of a token seed, the other servers from the ISO of the token.

| Scenario                                | Standalone servers     | Cluster members                 | Content                                                     |
|-----------------------------------------|------------------------|---------------------------------|-------------------------------------------------------------|
| `register_server`                       | IncusOS01              |                                 | Effects of the registration scriptlet                       |
| `bmc_power_off_and_on`                  | IncusOS01              |                                 | Power off and on through the BMC, see [Fake BMC](#fake-bmc) |
| `create_cluster`                        | IncusOS01 to IncusOS03 |                                 | Creates the cluster and asserts it                          |
| `cluster_rolling_reboot`                |                        | IncusOS01 to IncusOS03          | Rolling reboot with workload                                |
| `factory_reset_cluster`                 |                        | IncusOS01 to IncusOS03          | Factory reset with a token, which Operations Center creates |
| `create_cluster_from_template`          | IncusOS01 to IncusOS03 |                                 | Creates the cluster from a cluster template                 |
| `add_and_remove_server`                 | IncusOS04              | IncusOS01 to IncusOS03          | Adds IncusOS04 and removes IncusOS03                        |
| `factory_reset_cluster_with_token_seed` |                        | IncusOS01, IncusOS02, IncusOS04 | Factory reset with a given token and token seed             |
| `images_remote_launch_instance`         | IncusOS03              |                                 | Incus images, see [Incus images](#incus-images)             |
| `force_remove_lost_server`              |                        | IncusOS01, IncusOS02, IncusOS04 | Stops IncusOS04 and removes it from the cluster. Runs last. |

### `TestE2E_UpdateChannel`

The servers register with a token of the prod channel. The prod channel holds
the oldest update, so the servers have updates pending. Every scenario updates
its servers, so the pool installs them again before their next use.

| Scenario                    | Standalone servers | Cluster members        | Content                      |
|-----------------------------|--------------------|------------------------|------------------------------|
| `update_server_application` | IncusOS04          |                        | Updates a single application |
| `update_server_os_only`     | IncusOS04          |                        | Updates only the OS          |
| `cluster_rolling_update`    |                    | IncusOS01 to IncusOS03 | Rolling update with workload |

## Setup

A simple way to run the end to end tests as a developer is to create a VM using Incus
and run the tests inside the VM.

The tests create up to five virtual machines with a thin provisioned root disk of
`OPERATIONS_CENTER_E2E_TEST_DISK_SIZE` (50 GiB by default) each, plus one ISO
storage volume per Operations Center installation and per provisioning token.
Plan for at least 150 GiB on the storage pool. Since the tests rely on thin
provisioning, it is recommended to use a ZFS storage pool.

> **Note:**
> Running out of disk space during a run leaves the instances in `ERROR` state,
> from which they do not recover. `incus info <instance> --show-log` is the only
> place, where the cause is visible. Run `make clean-e2e-test-soft` between runs
> to reclaim the artifacts, which accumulate over the runs.

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
> With more advanced setups, the performance of the tests can be improved
> significantly. If this is a concern, consider using ZFS for the storage
> volume.
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
go test ./e2e_tests/ -v -timeout 240m -count 1 | tee e2e_tests.out
```

or

```shell
make e2e-test
```

or a specific test:

```shell
make e2e-test GO_TEST_RUN='^TestE2E_FromManualUpload_CreateCluster$'
```

or a single scenario of a suite:

```shell
make e2e-test GO_TEST_RUN='^TestE2E_OperationsCenter$/^oidc_authentication$'
```

to show all available test cases and scenarios:

```shell
make e2e-test-list
```

## Cleanup

Remove everything, including the Operations Center VM and the temporary
directory:

```shell
make clean-e2e-test
```

Remove the test artifacts, but keep the Operations Center VM and its ISO, which
are expensive to recreate. This is the one to run between test runs:

```shell
make clean-e2e-test-soft
```

Remove the log files accumulated by previous test runs:

```shell
make clean-e2e-test-logs
```

`make clean-e2e-test` and `make clean-e2e-test-soft` hide the errors for
resources that are already gone. They report all other errors, continue with
the remaining steps and fail at the end.

## Development

### Suites and scenarios

A suite is a top level test, which runs several scenarios with `runE2ESuite`.
Each scenario is a subtest, so `-run 'Suite/scenario'` selects it. Use
snake_case for the name of a scenario, since `go test` replaces spaces with
underscores.

The scenarios of a suite run in the given order and share the setup of the
Operations Center. The setup runs in the first scenario, which is selected by
`-run`, and again after a failed scenario. A scenario has its own timeout and
its own debug output on failure, like a top level test.

A suite can share a pool of IncusOS servers between its scenarios (see
`serverPool`). A scenario lists the servers it requires, either as standalone
servers or as the members of the cluster `incus-os-cluster`. The cluster uses
the update channel of the pool. Before the scenario runs, the pool reads the
state of the servers from Incus and from Operations Center.

The pool installs each server from its own ISO. This way, a suite covers
several ways of installing without a separate top level test. For example,
`TestE2E_StableServers` installs IncusOS01 from the ISO of a token seed.

For standalone servers, the pool decides for each server (see
`nextServerAction` and `planStandalone`):

* A server, which is ready, not clustered and registered with the name of its
  VM, is used as it is.
* A server, which registered with the hostname of its VM, is renamed. A server
  does this after a factory reset.
* A ready member of a cluster is recycled with a factory reset of the cluster.
  This takes about half the time of an install. The pool renames all the
  members after the reset.
* Any other server is removed and installed again. This covers a missing or
  stopped VM, a missing record and a server, which is not ready. Removing a
  clustered server removes its cluster, so the other members of this cluster
  are installed again as well.

A cluster is used as it is, if it consists of exactly the required servers and
all of them are ready (see `clusterReady`). Otherwise the pool brings the
servers into the standalone state first and then creates the cluster. The pool
adds the cluster as Incus remote for the duration of the scenario.

The servers of a failed scenario are installed again before their next use. A
scenario lists the servers in `leavesDirty`, if it changes them in a way the
pool does not observe. An example is the version of a server after an update.
The pool installs these servers again before their next use as well. The pool
keeps this state in memory only. After a run with
`OPERATIONS_CENTER_E2E_TEST_NO_CLEANUP`, remove the VMs of `TestE2E_UpdateChannel`
before running it again. Otherwise the pool reuses the updated servers.

The pool only installs servers, which a selected scenario requires, so a single
scenario only pays for its own servers. If it installs, it also installs all
the missing VMs of the pool, since concurrent installs take about as long as a
single one.

The pool removes its servers and its cluster when the suite ends, so a
scenario does not remove a cluster, which the next scenario can use. A
scenario still removes other resources it creates, e.g. its workload. Order the
scenarios so that each one leaves a state the next one can use. A scenario,
which leaves its servers unusable, runs last.

The other servers of the pool stay registered while a scenario runs. An
assertion therefore selects the servers by name. It does not expect a certain
number of servers in Operations Center, e.g. see
`assertRemovedServerToReappear`.

`go test -list` only shows the top level tests. With
`OPERATIONS_CENTER_E2E_TEST_LIST` set, a suite creates a skipped subtest for each
scenario and runs nothing else. `scripts/e2e/select.sh` reads the scenarios from
this output and creates a regex like `^Suite$/^scenario$|^OtherTest$`. The
alternatives are joined on the top level, since `go test` splits a regex at a
top level `/` into the levels of the subtests. The Makefile passes
`GO_TEST_RUN` with `$(value ...)`, so make does not expand the `$` anchors.

### Shell commands

The test helpers (`run`, `mustRun` and friends) execute their commands with
`bash -o pipefail`, so a command, which fails anywhere in a pipeline, fails the
whole command.

### Instance lifecycle and transient storage errors

Instances are not stopped, started, removed or restarted with `mustRun` directly.
Use `stopInstanceWithContext`, `startInstanceWithContext`,
`removeInstanceWithContext` and `restartInstanceWithContext` instead.

The helpers therefore treat the exit code as a weaker signal than the actual
state of the instance:

* `stopInstanceWithContext` tolerates a failed `incus stop`, as long as the
  instance does not report the status `Running` anymore. The tolerated failure
  is written to the test log.
* `startInstanceWithContext` and `removeInstanceWithContext` retry, if the
  output of the command matches one of the `transientStorageErrors`. Any other
  failure is returned immediately, so genuine errors are not retried.

A single attempt to shut an instance down cleanly is bounded by
`instanceStopTimeout`, so an instance, which can not shut down, e.g. because it
never booted an OS, fails the attempt instead of blocking for as long as Incus is
willing to wait. Since a tolerated stall costs up to that timeout, the timeouts
of `createIncusOSInstances` and of the cleanup registered by `cleanupIncusOS`
contain the corresponding headroom.

### Waiting for an IncusOS VM

`waitAgentRunningWithContext` and `waitExpectedLogWithContext` keep waiting until
the incus agent shows up, respectively until the wanted log line appears. Both
give up early on a state the VM can not recover from:

* The instance status `Error`, see `errUnrecoverableStatus`. The cause is on the
  host, most likely an exhausted storage pool or filesystem.
* A failed start of incus-osd, see `errIncusOSStartupFailure`. The known instance
  of this is incus-osd aborting the boot with `unable to configure incus-agent`,
  because restarting `incus-agent.service` failed. incus-osd exits in this case
  and IncusOS paints `!! IncusOS critical startup error !!` on the console, while
  the instance itself stays in the status `Running`, so neither the instance
  status nor `incus exec` reveals it. The console log is the only source.

  Since this is a flaky condition, the VM is restarted once and only then the
  test is failed. Restarting is not guaranteed to help, so the final error names
  the failure, even if the VM recovered but the incus agent stayed away.

  If incus-osd fails during the installation, `createIncusOSInstances` removes
  the VM and installs it once more from scratch. A restart in place does not
  help in this case, since the VM keeps its partially written disk and TPM
  state.

### Debug output

The debug output is collected in two places, both only written on a failing test:

* `logVMDebugInfo` writes to the test log. It collects the information for every
  VM at most once per test, since a failure propagated through an errgroup
  cancels and therefore fails every sibling goroutine as well. The output of each
  command is truncated, the console log through `sanitizeConsoleLog`, everything
  else through `tailMsg`.
* `onTestFailDebugOutput` writes the buffer filled by `debugf`, the journals and
  the complete console log of every VM to files in
  `OPERATIONS_CENTER_E2E_TEST_TMP_DIR`.

The console log of an IncusOS VM contains the redraws of its boot splash, which
is stripped of the terminal escape sequences and truncated by size, not by lines.

Commands, which are executed in a poll loop, use `runQuietWithContext` or
`mustRunQuiet`. Of a successful run only a summary is recorded, since a loop,
which polls e.g. a growing journal once per second for several minutes, would
otherwise fill the debug output with hundreds of copies of the very same log.
Failures are always recorded in full.

### Broken setup of the Operations Center

Every test and every scenario depends on `setupOperationsCenter`, so a failure
of it is terminal for the whole run.

`setupOperationsCenterOrMarkBroken` therefore records such a failure and the
remaining tests skip with that reason instead of reproducing it. Only the shared
setup is covered, a failure of the per test setup or of a test body is
independent and keeps its signal.

### Idempotent tests

The existing end to end tests are designed to be run individually as well as in
a sequence. This means, that each test case and each scenario should clean up
any resources it creates, so that the next one can run without interference.

This is achieved by using `t.Cleanup` to register cleanup functions that are
executed after the test case finishes, regardless of whether it passes or fails.

The cleanup functions should be designed to not fail if the resource they are
trying to clean up does not exist.

In most cases, the cleanup functions should be registered before the actual
resource is created, to ensure that they are executed even if the resource
creation is only partially successful.

The servers of a server pool are the exception within a suite. The pool
registers their cleanup on the suite, so the scenarios can share them.

The Operations Center VM is the exception, it is deliberately not cleaned up and
reused across test cases and test runs. `setupOperationsCenter` therefore only
reuses it, if it is running *and* its incus agent answers. A VM found in any
other state, or one, which is running but stuck, e.g. at the boot manager of a
boot media it can not boot, is removed and installed from scratch.
The same applies to left over `IncusOS0x` instances in `createIncusOSInstances`.

Examples:

* `t.Cleanup(clusterCleanup(t))`, cleans up any cluster created during the test
  case.
* `t.Cleanup(cleanupTokenSeed(t, token, name))`, cleans up the token seed created
  during the test case.
* `t.Cleanup(serverBMCConfigCleanup(t, tmpDir, name))`, resets the BMC config of
  a server, so that Operations Center stops talking to a BMC, which is about to
  go away.
* `t.Cleanup(ocIncusImageSourceCleanup(t, name))`, removes an Incus image
  source, which also removes all the images provided by that source.
* `t.Cleanup(systemSecurityOIDCAndOpenFGACleanup(t, tmpDir))`, resets the OIDC
  part and the OpenFGA part of the security config in a single update. The
  services behind them are about to go away. Operations Center rejects an
  update of one part if the other part points at a missing service. A failed
  run without cleanup leaves the config behind, so the tests also reset it at
  the start.

For debug purposes, the cleanup can be disabled by setting the environment
variable `OPERATIONS_CENTER_E2E_TEST_NO_CLEANUP` or
`OPERATIONS_CENTER_E2E_TEST_NO_CLEANUP_ON_ERROR` to a truthy value.

### Migratable workload

A rolling reboot and a rolling update migrate the workload of a server away and
back again. An instance, whose status is `Running`, is not necessarily migratable
yet: the wrapper, which fetches the incus agent binary, mounts the 9p share of
the agent in the guest, and for as long as it is mounted, QEMU refuses to migrate
the instance with `Migration is disabled when VirtFS export path ... is mounted
in the guest using mount_tag 'agent'`.

The wrapper unmounts the share before it execs the agent, and the agent only
answers afterwards, so `assertWorkloadRunning` waits for `incus exec` to succeed
for every instance of the workload before the run is triggered. The share is
mounted again whenever the agent restarts, so this narrows the window rather than
closing it.

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

`TestE2E_WithTokenSeed_DeployServer` covers the automated deployment of a
server. It creates an empty instance, pre-registers it with the proxy as its BMC
and has Operations Center deploy IncusOS on it. The BIOS attributes of the proxy
are the config keys of the instance, so the BIOS profile matching it is handed
in with the deployment (see `redfishProxyBIOSProfileYAMLTemplate`) and is not
part of the profiles shipped with Operations Center.

The proxy fetches the installation media from Operations Center when it is
attached, so Operations Center needs to be reachable from the end 2 end test
host at the address it advertises.

The proxy only answers the request to attach the media after it has imported
the media into Incus. This can take longer than a minute. The BIOS profile
therefore sets `attach_media_call_timeout` to 5 minutes. The value is lower
than the default, so a different error does not waste CI time. The test
stretches this value and the other deployment timeouts of the BIOS profile by
`OPERATIONS_CENTER_E2E_TEST_TIMEOUT_STRETCH_FACTOR`.

The proxy fetches the complete installation media with a single request. A BMC
streaming the media requests byte ranges instead. The test therefore reads the
installation media a second time, the way such a BMC does: the start
sequentially, then the end, then random portions (see
`assertVirtualMediaRandomRead`).

### Incus images

The scenario `images_remote_launch_instance` of the suite
`TestE2E_StableServers` covers the Incus image and the Incus image source
handling. It uploads images manually, syncs an image from
`https://images.linuxcontainers.org` through an image source, asserts the public
simplestreams endpoints of Operations Center and launches a container from a
manually uploaded and from a source synced image.

The scenario restarts the Operations Center daemon first. The first background
refresh of the image sources runs 10 minutes after the start of the daemon, so
it can not interfere with the assertions.

The filter expression of the image source is pinned to a single image and a
single version of that image, which is discovered from the image server at the
beginning of the test. This keeps the amount of downloaded data small and makes
the assertions on the synced images exact, even though the content of the image
server changes over time.

The Incus CLI caches the simplestreams responses of a remote for an hour, so the
test populates Operations Center completely before it registers Operations
Center as image remote on the server. Everything asserted after that point goes
through the public endpoints directly instead of through the server.

### Updates, channels and tokens

The scenario `provisioning_updates_channels_tokens` of the suite
`TestE2E_OperationsCenter` covers the updates, the channels and the tokens of
Operations Center. It needs no IncusOS server.

The test sends server registrations with curl. Each of these registrations
fails before a server record is created: the body has no server name, or the
token is exhausted, expired or unknown. This way the test checks the token
handling of the registration without a VM.

The test reads the ISO and the raw image of a public token seed with byte ranges
only, so the images are not downloaded. Operations Center still generates each
image completely, so the first request of an image takes a while.

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

### Fake ACME server

The ACME support of Operations Center is tested against
[Pebble](https://github.com/letsencrypt/pebble), the ACME test server of
Let's Encrypt. Pebble is served in-process by the test on an ephemeral port of
the end 2 end test host (see `startACMEProvider`), so no additional service
needs to be installed.

Pebble skips the validation of the challenges, so it needs no access to
Operations Center. The certificate of its directory is self signed and the root
CA of the issued certificates is new for every run. So neither is in any
default CA bundle, like with the internal ACME server of a company.

The test adds both CA certificates to the custom CA certificates of the IncusOS
host of Operations Center, before it deploys the servers. This way the seed of
the servers contains them as well.

The test changes the certificate of Operations Center. The CLI pins this
certificate, so the test adds the remote of the CLI again after every change.
On cleanup, the test disables ACME, restores the original certificate and the
custom CA certificates of the host.

### OpenFGA server

The OpenFGA authorization of Operations Center is tested against a real OpenFGA
server. The test starts OpenFGA as a process on the end 2 end test host (see
`startOpenFGA`).

The test downloads the OpenFGA release into the `openfga` folder of the
temporary directory (see `ensureOpenFGABinary`). It verifies the SHA-256 sum of
the download. The version and the SHA-256 sums are pinned in
`openfga_binary_test.go`. The test reuses an existing binary if it has the
pinned version. OpenFGA writes its log to `openfga.log` in the temporary
directory.

The download needs access to `github.com`. Only Linux hosts on amd64 and arm64
are supported. The pinned version is independent of the OpenFGA module version
in `go.mod`.

A test run without cleanup leaves OpenFGA running. Run
`make clean-e2e-test-soft` to stop it.

The HTTP API of OpenFGA listens on a free port on all the IPv4 addresses of
the end 2 end test host. Operations Center reaches OpenFGA at the address of the host on the
network of the Operations Center VM. Set
`OPERATIONS_CENTER_E2E_TEST_HOST_ADDRESS` to use a different address.

OpenFGA only authorizes users who authenticate through OIDC. The test
therefore also starts the fake OIDC provider. It uses an isolated config
directory for the `operations-center` CLI, so the remotes used by the other
tests stay untouched.

The test assigns one role after the other to the OIDC user by writing the
respective tuple to OpenFGA directly. For each role, it verifies which commands
of the `operations-center` CLI are allowed and which are denied.

The TLS based authentication keeps its unrestricted access. The test verifies
this at the end. Finally, the test verifies that Operations Center rejects an
OpenFGA server it cannot reach.
