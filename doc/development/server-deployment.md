# Server Deployment

## Overview

Deploying IncusOS onto a new machine is a sequence of BMC operations, which
takes 30 to 60 minutes and has to survive a restart of Operations Center. It is
driven by the automated deployment control loop, which is triggered with

```none
POST /1.0/provisioning/servers/{name}/:deploy
```

and stopped with

```none
POST /1.0/provisioning/servers/{name}/:cancel-deploy
```

Canceling interrupts the step in flight, so the clean up does not have to wait
out a BMC, that is not answering anymore. With `skip_cleanup`, the deployment
ends right away without the clean up, so the server is left untouched the way a
deployment, that failed on its own, is.

The deployment request carries the token and the token seed the installation
media is generated from, and optionally the virtual media device, the image
type, the architecture, which is taken from the BMC when it is not given, and
the channel. It deliberately carries **no BIOS attributes**: those are resolved
from the BIOS profiles matching the server.

The progress is reported through the server status and status detail, and in
more detail through the `deployment` field of the server.

## Pre-flight checks

Everything, that can be checked up front, is checked when the deployment is
requested, so an impossible deployment is rejected right away:

1. The server exists, is `unregistered` and has a BMC configured.
1. No deployment is already running for the server.
1. The token exists, has uses remaining and stays valid for the whole deployment
   timeout, since the server registers itself at the very end.
1. The token seed exists and is public.
1. The install config of the token seed sets `force_reboot`, so the server
   reboots on its own when the first stage of the installation is done. A seed
   without it is rejected unless the request sets `force`, in which case the
   deployment relies on the read progress of the installation media alone.
1. The public address of Operations Center is configured, since it is what the
   installation media URL handed to the BMC is built from.
1. A virtual media device is selected. If the request does not name one, the
   first device advertising CD or DVD support is picked, the devices offered by
   the system taking precedence over the ones offered by the manager. Only a BMC
   reporting no virtual media device at all rejects the request.
1. The architecture is settled, see below. Everything the deployment generates
   is built for it, so it is resolved before anything else is.
1. A deployment, that asks for the secure boot enrollment media, is rejected
   where the BMC reports no secure boot mode: Operations Center could then
   neither put the server into the setup mode the media needs nor tell, whether
   the enrollment worked.
1. A deployment, that asks for the secure boot enrollment media, is rejected
   where the architecture of the server is not `x86_64`, which is the only
   architecture the media can be built for, see below.
1. The BIOS profiles matching the server resolve to something. The resolved
   profile names, attributes, deferred attributes and secure boot allow lists
   are snapshotted onto the deployment, so a later change of the catalog does
   not alter what a running deployment applies.

### Architecture

The architecture is taken from the BMC as primary source.
`BMCData.ServerArchitecture` reads it from what the BMC reports
about the first processor, preferring the instruction set, which is the only one
of the two properties, that carries the bitness — Redfish has no 64 bit x86
architecture, a 64 bit x86 processor reports the architecture `x86`.

The architecture of the request is therefore an **override**, not the source:

* It is filled in from the BMC, when the request does not name one, and the
  resolved value is what the deployment is persisted with, so everything built
  later — the installation media and the secure boot enrollment media — is built
  for the same architecture.
* A requested architecture, that contradicts the BMC, is rejected.
* A request naming none for a server, whose BMC reports nothing to go by, is
  rejected. Naming one explicitly is the escape hatch for such a BMC, and is
  accepted with a warning, since it can not be confirmed.

## State machine

The state of the deployment lives in `Server.StatusInternal.Deployment`, which
is stored as JSON on the server record and is not part of the REST API surface.
Two rules make the state machine restart safe:

* **Every step is split into a trigger state and a wait state.** A retry
  re-issues the trigger, a wait timeout falls back to it, and a daemon restart
  re-enters the persisted state.
* **Every wait condition is re-derivable from the BMC data or the server record
  alone** — the power state, the BIOS attribute values,
  `BMCData.VirtualMedia[].Inserted` and `Server.Status`. A Redfish task monitor
  404s once it has been consumed or after a BMC reset, so the persisted monitor
  URI is an optimization, never the source of truth.

Both rules rest on **every action being idempotent**: a crash between entering a
trigger state and the BMC having accepted the operation leaves the deployment in
the trigger state, so the action is simply issued again.

```{mermaid}
stateDiagram-v2
    state "refresh BMC data" as RefreshBMCData
    state "check BIOS" as CheckBIOS
    state "power off" as PowerOffBIOS
    state "wait for power off" as WaitPowerOffBIOS
    state "apply BIOS" as ApplyBIOS
    state "power on" as PowerOnBIOS
    state "wait for BIOS applied" as WaitBIOSApplied
    state "verify BIOS" as VerifyBIOS
    state "power off" as PowerOffBIOSDeferred
    state "wait for power off" as WaitPowerOffBIOSDeferred
    state "apply deferred BIOS" as ApplyBIOSDeferred
    state "power on" as PowerOnBIOSDeferred
    state "wait for BIOS applied" as WaitBIOSAppliedDeferred
    state "verify deferred BIOS" as VerifyBIOSDeferred
    state "power off" as PowerOffSecureBoot
    state "wait for power off" as WaitPowerOffSecureBoot
    state "secure boot certificates" as SecureBoot
    state "reset secure boot keys" as ResetSecureBootKeys
    state "wait for key databases cleared" as WaitSecureBootReset
    state "power on" as PowerOnSecureBootReset
    state "wait for setup mode" as WaitSecureBootSetupMode
    state "power off" as PowerOffSecureBootReset
    state "wait for power off" as WaitPowerOffSecureBootReset
    state "attach enrollment media" as AttachSecureBootMedia
    state "wait for media attached" as WaitSecureBootMediaAttached
    state "power on" as PowerOnSecureBootMedia
    state "wait for certificates enrolled" as WaitSecureBootEnrolled
    state "power off" as PowerOffSecureBootMedia
    state "wait for power off" as WaitPowerOffSecureBootMedia
    state "clear stale media" as ClearMedia
    state "wait for media cleared" as WaitMediaCleared
    state "enable secure boot" as EnableSecureBoot
    state "power on" as PowerOnSecureBoot
    state "wait for secure boot settled" as WaitSecureBootSettled
    state "power off" as PowerOffSecureBootSettled
    state "wait for power off" as WaitPowerOffSecureBootSettled
    state "attach media" as AttachMedia
    state "wait for media attached" as WaitMediaAttached
    state "power on" as PowerOnInstall
    state "installing" as WaitInstall
    state "detach media" as DetachMedia
    state "wait for media detached" as WaitMediaDetached
    state "wait for reboot" as WaitReboot
    state "wait for registration" as WaitRegistration
    state "cleanup" as Cleanup
    state "cancel" as Cancel
    state "wait for power off" as WaitCancel
    state "completed" as Completed
    state "failed" as Failed
    state "canceled" as Canceled

    [*] --> RefreshBMCData: deploy triggered
    RefreshBMCData --> CheckBIOS
    CheckBIOS --> PowerOffBIOS: attributes not applied
    CheckBIOS --> PowerOffBIOSDeferred: attributes match
    CheckBIOS --> PowerOffSecureBoot: attributes match, no deferred attributes pending
    PowerOffBIOS --> WaitPowerOffBIOS
    WaitPowerOffBIOS --> PowerOffBIOS: timeout
    WaitPowerOffBIOS --> PowerOffBIOS: server powered on again
    WaitPowerOffBIOS --> ApplyBIOS: power state settled off
    ApplyBIOS --> PowerOnBIOS
    PowerOnBIOS --> WaitBIOSApplied
    WaitBIOSApplied --> PowerOffBIOS: timeout
    WaitBIOSApplied --> VerifyBIOS: task completed, or task unavailable after settle delay and power state on
    VerifyBIOS --> PowerOffBIOS: attributes not applied
    VerifyBIOS --> PowerOffBIOSDeferred: attributes match
    VerifyBIOS --> PowerOffSecureBoot: attributes match, no deferred attributes pending
    PowerOffBIOSDeferred --> WaitPowerOffBIOSDeferred
    WaitPowerOffBIOSDeferred --> PowerOffBIOSDeferred: timeout
    WaitPowerOffBIOSDeferred --> PowerOffBIOSDeferred: server powered on again
    WaitPowerOffBIOSDeferred --> ApplyBIOSDeferred: power state settled off
    ApplyBIOSDeferred --> PowerOnBIOSDeferred
    PowerOnBIOSDeferred --> WaitBIOSAppliedDeferred
    WaitBIOSAppliedDeferred --> PowerOffBIOSDeferred: timeout
    WaitBIOSAppliedDeferred --> VerifyBIOSDeferred: task completed, or task unavailable after settle delay and power state on
    VerifyBIOSDeferred --> PowerOffBIOSDeferred: attributes not applied
    VerifyBIOSDeferred --> PowerOffSecureBoot: attributes match
    PowerOffSecureBoot --> WaitPowerOffSecureBoot
    WaitPowerOffSecureBoot --> PowerOffSecureBoot: timeout
    WaitPowerOffSecureBoot --> PowerOffSecureBoot: server powered on again
    WaitPowerOffSecureBoot --> SecureBoot: power state settled off
    WaitPowerOffSecureBoot --> ClearMedia: power state settled off, secure boot certificates skipped
    WaitPowerOffSecureBoot --> ResetSecureBootKeys: power state settled off, enrollment media requested
    SecureBoot --> ClearMedia
    ResetSecureBootKeys --> WaitSecureBootReset: key databases cleared, BMC runs a task
    ResetSecureBootKeys --> PowerOnSecureBootReset: key databases cleared
    ResetSecureBootKeys --> AttachSecureBootMedia: already in setup mode
    WaitSecureBootReset --> PowerOnSecureBootReset: BMC task done
    PowerOnSecureBootReset --> WaitSecureBootSetupMode
    WaitSecureBootSetupMode --> PowerOnSecureBootReset: timeout
    WaitSecureBootSetupMode --> PowerOffSecureBootReset: secure boot mode is setup mode
    PowerOffSecureBootReset --> WaitPowerOffSecureBootReset
    WaitPowerOffSecureBootReset --> PowerOffSecureBootReset: timeout
    WaitPowerOffSecureBootReset --> PowerOffSecureBootReset: server powered on again
    WaitPowerOffSecureBootReset --> AttachSecureBootMedia: power state settled off
    AttachSecureBootMedia --> WaitSecureBootMediaAttached
    WaitSecureBootMediaAttached --> AttachSecureBootMedia: timeout
    WaitSecureBootMediaAttached --> PowerOnSecureBootMedia: enrollment media inserted in selected device
    PowerOnSecureBootMedia --> WaitSecureBootEnrolled
    WaitSecureBootEnrolled --> PowerOnSecureBootMedia: timeout
    WaitSecureBootEnrolled --> PowerOffSecureBootMedia: secure boot mode left setup mode, or reboot detected
    PowerOffSecureBootMedia --> WaitPowerOffSecureBootMedia
    WaitPowerOffSecureBootMedia --> PowerOffSecureBootMedia: timeout
    WaitPowerOffSecureBootMedia --> PowerOffSecureBootMedia: server powered on again
    WaitPowerOffSecureBootMedia --> ClearMedia: power state settled off
    ClearMedia --> WaitMediaCleared
    WaitMediaCleared --> ClearMedia: timeout
    WaitMediaCleared --> EnableSecureBoot: no media inserted
    EnableSecureBoot --> PowerOnSecureBoot
    WaitMediaCleared --> AttachMedia: no media inserted, secure boot certificates skipped
    PowerOnSecureBoot --> WaitSecureBootSettled
    WaitSecureBootSettled --> PowerOnSecureBoot: timeout
    WaitSecureBootSettled --> PowerOffSecureBootSettled: reboot detected or settle duration passed
    PowerOffSecureBootSettled --> WaitPowerOffSecureBootSettled
    WaitPowerOffSecureBootSettled --> PowerOffSecureBootSettled: timeout
    WaitPowerOffSecureBootSettled --> PowerOffSecureBootSettled: server powered on again
    WaitPowerOffSecureBootSettled --> AttachMedia: power state settled off
    AttachMedia --> WaitMediaAttached
    WaitMediaAttached --> AttachMedia: timeout
    WaitMediaAttached --> PowerOnInstall: expected media inserted in selected device
    PowerOnInstall --> WaitInstall
    WaitInstall --> DetachMedia: install stage 1 done
    DetachMedia --> WaitMediaDetached
    WaitMediaDetached --> DetachMedia: timeout
    WaitMediaDetached --> WaitReboot: media ejected
    WaitReboot --> WaitRegistration: server registered, reboot detected, or observation window passed while powered on
    WaitRegistration --> Cleanup: server registered
    Cleanup --> Completed
    Completed --> [*]

    RefreshBMCData --> Failed: retries exhausted
    WaitInstall --> Failed: timeout
    WaitReboot --> Failed: timeout
    WaitRegistration --> Failed: timeout
    Failed --> [*]

    RefreshBMCData --> Cancel: cancel requested
    WaitInstall --> Cancel: cancel requested
    WaitInstall --> Canceled: cancel requested, clean up skipped
    Cancel --> WaitCancel
    WaitCancel --> Cancel: timeout
    WaitCancel --> Canceled: power state off
    Canceled --> [*]
```

To keep the diagram readable, only some of the edges into `failed` and `cancel`
are drawn. Every state can reach both: a trigger, that exhausts its retry
budget, and a wait, that has no fallback and times out, end in `failed`, and a
cancellation preempts every state but the clean up it triggers itself. A
cancellation, that skips the clean up, has nothing to trigger and goes straight
to `canceled`.

### BIOS attributes are applied in two passes

Firmware rejects an attribute, whose prerequisite has merely been staged rather
than being in effect. A BIOS profile separates such an attribute out into
`deferred_attributes`, which the deployment applies in a second pass — another
power off, application, power on and verification — once the attributes of the
first pass are in effect.

**Either pass is passed by, when the server is configured correctly already**,
which is the common case for a server, that is deployed a second time. The
`check BIOS` state reads the attributes back before the first pass and records
for both passes, whether they still have anything to apply. An attribute, that
the BMC does not report at all, keeps its pass pending: not being able to tell
has to run the pass, never skip it. What the check records for the second pass is
only a first estimate, since a deferred attribute is only published once the
attribute, it depends on, is in effect — the verification of the first pass
overwrites it, whenever that pass runs.

**An attribute, whose value depends on the secure boot state, is verified after
the certificates are enrolled.** Both passes run before any secure boot work, and
the enrollment takes the server through the secure boot setup mode. A mismatch on
a server, that is not in setup mode, fails its pass right away, the way it always
has.

### Secure boot certificates

The server is powered off before the certificates are enrolled and stays off
until it is booted from the installation media: most firmware only accepts the
modification of the UEFI key databases while the server is powered off, and the
enrolled certificates only take effect on the next power on.

**A key database, that is enrolled correctly already, is left untouched.** Every
database is read before it is written and passed by, when it holds the
certificates of IncusOS plus the allow listed entries and nothing else. Anything,
that can not be told apart, has it reinitialized. The databases are judged one by
one, so a correct KEK is left alone even when the dbx has to be rewritten. What
survives the wipe is named by the `secure_boot` section of the BIOS profiles.

**The firmware is given a boot of its own to pick the certificates up.** It
applies them during the POST that follows the enrollment and then reboots the
server on its own. Letting that happen on the install boot would put a firmware
reboot right where the reboot, that ends the first stage of the installation, is
looked for. The deployment therefore boots the server once with no media
attached, waits for the reboot, and powers it off again before the installation
media is attached. Not every firmware reboots, so the wait settles itself after
`ServerDeploymentSecureBootSettleDuration`, and it is passed by entirely, where
the enrollment wrote to no key database at all.

**Not every BMC lets the UEFI key databases be modified through its Redfish
API.** A deployment for such a server either sets `secure_boot_enrollment_media`,
which enrolls the certificates by booting a generated image instead, see below,
or `skip_secure_boot_certificates`, which passes the `secure boot certificates`
state by and leaves the enrollment to an operator beforehand. The power off keeps
its place in every case, since the server has to be off for the media to be
attached.

### Enrolling from the secure boot enrollment media

A BMC, that refuses to write the individual key databases, usually does let them
be **cleared**: `SecureBoot.ResetKeys` is far more widely implemented than the
modification of the databases. Clearing them puts the server into the secure boot
**setup mode**, where the firmware accepts a key database update without checking
its signature, which is what the enrollment media relies on.

`secure_boot_enrollment_media` therefore replaces the single
`secure boot certificates` state with two passes, entered from the same power
off, and rejoining the common path at `clear stale media`:

1. **The reset pass** — `reset secure boot keys` clears the key databases through
   the BMC, then the server is booted once and powered off again. A BMC, that
   answers the reset with a task of its own, has it awaited by
   `wait for key databases cleared` first, so the server is not powered on while
   the databases are still being cleared. The reset is a no-op for a server, that
   reports the setup mode already, and the boot is passed by entirely in that
   case.
1. **The enrollment pass** — `attach enrollment media` generates the image and
   attaches it as the boot device, the server boots it once, `systemd-boot`
   enrolls the key databases, and the server is powered off again.

**Each pass gets a boot of its own**, for the same reason the Redfish enrollment
gets its settle boot: the firmware picks a cleared key database up during the
POST that follows, so the reset has to be in effect before the enrollment media
is booted.

**Both waits around the boots are satisfied from the secure boot mode the BMC
reports.** The `wait for setup mode` ends when the mode is `SetupMode`, and the
`wait for certificates enrolled` when it is anything else again.

The `wait for key databases cleared` is the exception: there is no observable
condition while the server stays powered off, so it follows the BMC task and
gives up on it once the BMC has forgotten it.

**The enrollment media is attached to the virtual media device of the
deployment**, the same one the installation media uses later, and is ejected by
`clear stale media` before the installation media is attached. Nothing else
distinguishes it from the installation media as far as the BMC is concerned.

**Only certificate material is enrolled again.** The allow list of the BIOS
profiles is resolved to the certificates of the catalog, while an allow listed
signature is reported as a warning and lost: the media enrolls signature lists
built from certificates, so there is nothing to rebuild a bare signature from.

Since the enrollment media enrolls the certificates itself, the deployment never
sets `SecureBootPending`, so the settle boot of the Redfish path is passed by and
the deployment continues straight to `attach media`.

### Building the secure boot enrollment media

Where the key databases can not be written through the Redfish API, the
certificates are enrolled by booting a generated image instead.
The image holds nothing but an **unsigned** copy of `systemd-boot` and the `PK`,
`KEK`, `db` and `dbx` updates in `loader/keys/auto/`, with
`secure-boot-enroll force` in its `loader.conf`. Being unsigned, it only ever
boots on a server whose secure boot is in setup mode, where it enrolls the
certificates, or disabled, where it does nothing at all.

**The media is only built for `x86_64` servers.** The image holds the boot loader
of the architecture it boots, `systemd-bootx64.efi`, and `systemd-boot-efi` only
ships the boot loader of the architecture it is installed for, which is the
`x86_64` Operations Center is deployed on. A deployment of a server of any other
architecture is therefore rejected when it is requested.

**The platform key is the one database, that is not built here.** It is taken from
`/boot/loader/keys/auto/PK.auth` of the Operations Center host, where IncusOS puts
its own signed update, and copied onto the media as it is. Firmware, that requires
the platform key to be self signed, verifies the update against the very
certificate it carries, so only the update signed by the platform key of IncusOS
is accepted, and Operations Center does not hold that key. Rebuilding it here signs
it with the throw away key below and has such a firmware refuse the enrollment with
a security violation, while firmware, that does not require it, accepts either.

**The generation shells out** to `cert-to-efi-sig-list` and `sign-efi-sig-list`
(`efitools`), `mkfs.vfat` (`dosfstools`), `mcopy` (`mtools`) and
`systemd-repart`, and takes the boot loader from `systemd-boot-efi`. All of them
are checked when the deployment is requested. The key database updates are
signed with a throw away key generated per image: their signature is never
checked, since the enrollment of db, dbx and KEK only happens in setup mode, and
the certificate ending up enrolled is the one inside the signature list, never the
one signing it. The platform key is the exception, see above.

**The partition table is laid out for the block size the media is read in**,
which follows the image type of the deployment: 2048 bytes for an `iso`, which a
BMC presents as a CD, and 512 bytes for a `raw` image, which it presents as a USB
stick. A table laid out for the other one is not found by the firmware at all, so
the enrollment media is built for the same image type as the installation media,
and a server, that boots only one of the two, boots both of them. The block size
is why `systemd-repart --sector-size=` writes the table: `sgdisk` takes the sector
size from the device it is pointed at and would need the image to be attached as
a loop device to write anything but a 512 byte one. `systemd-repart` writes it to
a regular file, so the generation needs no privileges, and every identifier of
the table is derived from `--seed=`, which keeps the very same certificates
producing the very same image.

**A generated media is cached and pruned, but never out from under a BMC.** The
image is addressed by a fingerprint of what it holds, so asking for the same
certificates twice serves the same file, and a background task removes what has
not been read within the TTL. A media, that the deployment of a server still
names, is kept however old it is: a BMC reads an attached media only when the
server boots, so the TTL says nothing about whether it is still needed, and
removing it would leave the BMC pointed at a URL answering "404 Not Found". The
last access is recorded on the file itself as well as in memory, so it survives a
restart — an image, that only the memory knew about, would fall back to the time
it was generated at and be pruned right away.

### Power and failure handling

**The deployment cuts the power rather than asking for a graceful shutdown.** A
graceful shutdown is an ACPI request, which only an operating system answers,
while a server, that is being deployed, sits in its POST or in the installer: the
BMC accepts the request and the server quietly ignores it.

**A power off is only done once it has settled.** A single power state of `Off`
does not establish it: firmware, that resets the server on its own to pick up
what has been staged for it, has the BMC report the power state off in the trough
of that reset, and the server is back in its POST moments later, where it accepts
neither a boot source override nor a modification of its key databases. Every
power off wait therefore only passes once the BMC has been reporting the server
off for `ServerDeploymentPowerOffSettleDelay`. A server, that is found powered on
again after it had been reported off, reverts the deployment to the power off
rather than the wait being left to time out. A wait only ever observes the
server, it never acts on it: every power operation is a state of its own, so
the generated diagram shows every one of them. The clean up of a canceled deployment is the exception: nothing follows it,
which the server would have to be down for.

**A BMC, that turns a request down because the server is not settled**, is
answered by going back to that power off rather than by failing the deployment. A
rejection like the `UnableToModifyDuringSystemPOST` of an iLO is not the BMC
refusing the request, it is the request having been issued too early, so the
deployment re-establishes what the request needs. It shares the `FallbackAttempts`
budget with the BIOS verification, so a server, that never settles, still ends the
deployment instead of looping.

**A failed deployment is not cleaned up.** The installation media stays attached
and the power state is left as it is, so an operator can look at the server
through the BMC console. A canceled deployment, in contrast, does eject the
media and power the server off, unless the cancellation sets `skip_cleanup`, in
which case the server is left untouched as well.

## Booting the installation media

The installation media is attached to a virtual media device of the BMC and the
next boot of the server is pointed at it. A **one time** override is preferred
and a **continuous** one is only used where the BMC turns the one time override
down. Which of the two the BMC accepted is logged when the media is attached,
since the two behave differently beyond the request: a one time override is spent
on the boot it was set for, so the reboot at the end of the installation follows
the boot sequence of the server again, while a continuous override stays armed
and sends the server straight back into the installation media.

Neither of them removes the media from the boot sequence, so **what keeps the
server out of the installer is ejecting the media**, which the deployment does as
soon as it has detected the end of the first stage. Detaching is also what takes
the override back, so the device the media was attached to is detached even when
the BMC reports nothing inserted in it any more.

## Detecting the end of install stage 1

The first stage of the IncusOS installation ends without any way for the machine
to report it: the installer returns before networking is brought up, so no phone
home is possible. The deployment therefore layers the signals and takes whichever
fires first.

1. **The server registered itself.** `POST /1.0/provisioning/servers?token=…`
   already happened, so the machine rebooted and finished on its own. This is
   the authoritative signal.
1. **A reboot was detected**, by comparing `BMCData.ServerLastResetTime` and
   `BMCData.ServerBootProgress` against a snapshot. Not every BMC reports these
   properties, and the helper then reports "cannot tell" rather than "not
   rebooted", so the deployment falls through to the next signal.
1. **Enough of the installation media has been read and it has gone idle** for
   `ServerDeploymentMediaIdlePeriod`. Operations Center serves the media itself
   and records the read progress per deployment. Only a deployment requested with
   `force` is told by this signal.
1. **An overall timeout** as a backstop, which fails the deployment rather than
   guessing.

The snapshot signal 2 measures against is only taken once the server is powered
on and a settle delay has passed, so the power on, that starts the installation,
is not mistaken for the reboot at the end of it. A boot progress state entered
after the snapshot only counts as a reboot, if it is one the boot has already
been past, since a server running through its POST enters one state after the
other, which is a boot making progress rather than a new one.

Signal 3 is skipped when the BMC negotiated `TransferMethod: Upload`, since such
a BMC fetches the whole image before the server even boots, and when no progress
has been recorded at all, which is the case after a daemon restart, since the
progress is kept in memory only.

**Signal 2 is gated on the installer having been running**, so that the reboot,
that ends the first stage, is not confused with the one the firmware performs to
pick up staged BIOS attributes or enrolled secure boot certificates. The latter
comes within the POST cycles of the very boot, that is supposed to start the
installer, so what separates the two is `BootProgress`: only a boot, that got as
far as `OSBootStarted`, ran the installer. Where the reboot does not end the
wait, it re-anchors the snapshot instead, so it is not carried into the next
comparison. `ServerDeploymentInstallRebootFallbackDelay` stands in for the boot
progress where the BMC does not report it, and is the only case, in which time
decides.

**Ejecting has to be prompt.** A server, whose media is still attached when it
comes back up, can boot it a second time rather than the system just installed,
where the installer finds no install target anymore and the server never
registers. The POST of that reboot is the whole budget, which is why signal 2
must not wait out a duration once it has seen the installer run.

**The read progress tells neither signal apart**, since it does not tell the
installer streaming the media apart from the BMC pulling it in: a BMC, that
caches the media rather than handing every read through, reads the whole image
out while the server boots and then goes quiet for the rest of the installation,
which is exactly what "read out and idle" is supposed to mean. A Dell PowerEdge
does this, and ejecting the media on that signal breaks the installation with an
I/O error on the virtual CD.

**Signal 3 is therefore reserved for `force`** — a token seed without
`force_reboot` — where the installer waits for the media to disappear instead of
rebooting, so signal 2 does not fire on its own and ejecting the media is what
makes IncusOS reboot. A server, that reboots on its own, is told by signals 1
and 2 alone.

There, `ServerDeploymentMediaIdlePeriod` is the whole latency of the deployment:
the server has installed and is idling in front of "Please remove the install
media", and nothing happens until the ejection comes. The period therefore only
covers a gap within the installation — a Lenovo ThinkSystem reads the media right
up to the end of the first stage — and is not a place to buy safety margin.
`ServerDeploymentMinInstallDuration` keeps the floor underneath it, so a BMC,
that read the media out while the server was still booting, can not end the wait
before the installation could have run at all.

### Read progress of the installation media

**How far into the image the reader has got says nothing on its own.** A boot
loader looks at the partition table at the very end of the image within its first
few requests, which puts the whole image behind the reader seconds after the
power on, while next to nothing has been read. What tells the installation apart
from the boot is the amount actually read, so
`ServerDeploymentMediaMinBytesRead` — 500 MiB, capped at the size of the image —
has to have been served before the media going idle counts. It is an absolute
amount rather than a share, since how much of an image the installer fetches is a
property of the image and of the installer.

**What counts is the distinct bytes served, not the bytes handed out.** A BMC
re-requests ranges it has fetched before, so summing every byte handed out
reaches the bar without the installer ever having streamed the image. The
progress is therefore kept as the set of the ranges served, merged as they
arrive, and capped at a bounded number of ranges. Reaching the cap coalesces the
two neighbors separated by the smallest gap, which over counts rather than under
counts, since under counting would leave the deployment waiting for a read, that
has already happened. `media_bytes_read` of the deployment status reports that
amount.

**The progress is dropped when the media is attached, when the deployment cleans
up and when it is canceled**, so that neither a retry nor a later deployment
inherits what an earlier one left behind. A **failed** deployment keeps its
record on purpose, for the same reason it keeps its media attached.

**A BMC does not necessarily read the media from the address its Redfish API is
reached at**, so the progress is dropped by image rather than by source address,
and a deployment, that finds nothing recorded for the address of its BMC, falls
back to the only source reading its image, if there is exactly one. Several
sources are several servers, which can not be told apart any more, and the
deployment reports "cannot tell": `media_bytes_read` is `-1`, which
`server deploy-status` shows by omitting the read progress.

## Waiting for the reboot

Once the media has been ejected, the deployment waits for the server to come
back up. **A server, that is merely powered on, has not necessarily rebooted** —
it can just as well still be running an installation, that was mistaken for a
finished one — so the wait looks for an actual reboot. It measures against the
snapshot taken for the install wait, not against a fresh one: the reboot, that
ended that wait through signal 2, has already happened by the time this state is
entered.

A server, that powered off instead of rebooting, reverts the deployment to
**power on**, which leads back to the wait. Only BMC data collected after the
wait has been entered counts, so the power on, that led to it, is not mistaken
for the server having stayed off. A BMC, that reports none of the properties the reboot detection needs, can
never answer the question, so `ServerDeploymentRebootObservationWindow` bounds
how long the reboot is looked for before the wait settles for the server not
being powered off. The state has no fallback, so timing out would fail the
deployment.

## Retries and timeouts

One dispatcher enforces both, driven by the state table, which holds the kind of
every state, its timeout, the trigger a wait falls back to and the successor.

* **Trigger state**: on a retryable error within the retry budget, the counter
  is incremented, the state is kept and the next attempt is gated on an
  exponential backoff. The generated error wrapper middleware already marks BMC
  transport errors as retryable. A non-retryable error or an exhausted budget
  fails the deployment.
* **Wait state**: the condition being met advances the deployment and resets the
  counter. Within the timeout, the deployment simply stays. On a timeout the
  deployment falls back to the trigger, which counts against the retry budget of
  the wait in `WaitRetries`. The per state counter can not hold that budget,
  since the trigger succeeding resets it, while `WaitRetries` survives the round
  trip and is only reset once a wait is met.
* **Revert**: a wait, that observes something contradicting the step before it —
  a server powered on again after a power off, or a server, that stayed off
  after the installation — reverts the deployment to the action, that
  re-establishes it. Reverts share the `WaitRetries` budget with the timeouts of
  the wait.
* **Call timeout**: independently of both, every attempt of a state runs with a
  deadline of its own, so a BMC, that accepts the connection and then stops
  answering, ends the attempt instead of parking the control loop. Running out
  of it is retryable, so a trigger is issued again and a wait is simply
  evaluated again. Four actions get a budget of their own, since their legitimate
  duration is minutes rather than seconds: **attach media** and
  **attach enrollment media**, which a BMC, that uploads the media instead of
  streaming it, only answers once it has read the whole image, and
  **secure boot** and **reset secure boot keys**, which touch every entry of the
  key databases with a request of its own.
* **BIOS re-application**: a verification, that finds an attribute at the wrong
  value, and a BIOS wait, that times out, both route the deployment back to the
  **power off** of their pass rather than to the application itself, since the
  firmware only picks the staged attributes up on a reset. The verification
  carries its own budget, `FallbackAttempts`, shared by both passes: the state it
  returns to succeeds every time, which would reset the per state counter on
  every round and let the two states hand the deployment back and forth forever.
* **Global**: the overall duration is checked at the top of every tick.

The defaults live in `internal/config/daemon/consts.go`.

## Control loop

The control loop is registered as a background task next to the cluster update
control loop and runs on `ServerDeploymentControlLoopInterval`.

* It selects the servers with a deployment in progress through the **indexed**
  status filters only. A server stays in status `deploying` for all of the
  deployment but the very last steps, which run after the server has registered
  itself and therefore moved on to `pending` / `registering`, which the loop
  selects as well.
* The deployments are advanced **concurrently**, bounded by
  `ServerDeploymentControlLoopConcurrency`. Every step blocks on the BMC of its
  server, so a BMC, that is not answering, must not keep the other servers from
  moving on.
* A per server mutex, taken with `TryLock`, keeps the timer driven and the event
  driven entries from interleaving on a single server. An entry, that finds the
  deployment being advanced already, hands its trigger over to the holder, which
  picks it up before it returns, so an event, that satisfies the wait the
  deployment is in, is not waited out by the next tick.
* **Run until blocked**: after a trigger has succeeded, the wait state it enables
  is evaluated in the same tick, bounded by
  `ServerDeploymentMaxTransitionsPerTick`, so one wedged deployment can not hog
  the tick. The same budget bounds the handed over triggers.
* It is additionally triggered, scoped to a single server, when a deployment is
  created, on `ServerLifecycleSignal`, so a registration resolves in seconds,
  and on `BMCVirtualMediaSignal`, so attaching and detaching the installation
  media resolves promptly.

see also [Status values](/reference/server)
