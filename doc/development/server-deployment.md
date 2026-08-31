# Server Deployment

## Overview

Deploying IncusOS onto a new machine is a sequence of BMC operations, which
takes 30 to 60 minutes and has to survive a restart of Operations Center. It is
driven by the automated deployment control loop, which is triggered with

```
POST /1.0/provisioning/servers/{name}/:deploy
```

and stopped with

```
POST /1.0/provisioning/servers/{name}/:cancel-deploy
```

Canceling interrupts the step in flight, so the clean up does not have to wait
out a BMC, that is not answering anymore.

The deployment request carries the token and the token seed the installation
media is generated from, and optionally the virtual media device, the image
type, the architecture and the channel. It deliberately carries **no BIOS
attributes**: those are resolved from the BIOS profiles matching the server.

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
1. The BIOS profiles matching the server resolve to something. The resolved
   profile names, attributes, deferred attributes and secure boot allow lists
   are snapshotted onto the deployment, so a later change of the catalog does
   not alter what a running deployment applies.

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
    state "clear stale media" as ClearMedia
    state "wait for media cleared" as WaitMediaCleared
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
    WaitPowerOffBIOS --> ApplyBIOS: power state off
    ApplyBIOS --> PowerOnBIOS
    PowerOnBIOS --> WaitBIOSApplied
    WaitBIOSApplied --> PowerOffBIOS: timeout
    WaitBIOSApplied --> VerifyBIOS: task completed, or task unavailable after settle delay and power state on
    VerifyBIOS --> PowerOffBIOS: attributes not applied
    VerifyBIOS --> PowerOffBIOSDeferred: attributes match
    VerifyBIOS --> PowerOffSecureBoot: attributes match, no deferred attributes pending
    PowerOffBIOSDeferred --> WaitPowerOffBIOSDeferred
    WaitPowerOffBIOSDeferred --> PowerOffBIOSDeferred: timeout
    WaitPowerOffBIOSDeferred --> ApplyBIOSDeferred: power state off
    ApplyBIOSDeferred --> PowerOnBIOSDeferred
    PowerOnBIOSDeferred --> WaitBIOSAppliedDeferred
    WaitBIOSAppliedDeferred --> PowerOffBIOSDeferred: timeout
    WaitBIOSAppliedDeferred --> VerifyBIOSDeferred: task completed, or task unavailable after settle delay and power state on
    VerifyBIOSDeferred --> PowerOffBIOSDeferred: attributes not applied
    VerifyBIOSDeferred --> PowerOffSecureBoot: attributes match
    PowerOffSecureBoot --> WaitPowerOffSecureBoot
    WaitPowerOffSecureBoot --> PowerOffSecureBoot: timeout
    WaitPowerOffSecureBoot --> SecureBoot: power state off
    WaitPowerOffSecureBoot --> ClearMedia: power state off, secure boot certificates skipped
    SecureBoot --> ClearMedia
    ClearMedia --> WaitMediaCleared
    WaitMediaCleared --> ClearMedia: timeout
    WaitMediaCleared --> PowerOnSecureBoot: no media inserted
    WaitMediaCleared --> AttachMedia: no media inserted, no certificates enrolled
    PowerOnSecureBoot --> WaitSecureBootSettled
    WaitSecureBootSettled --> PowerOnSecureBoot: timeout
    WaitSecureBootSettled --> PowerOffSecureBootSettled: reboot detected or settle duration passed
    PowerOffSecureBootSettled --> WaitPowerOffSecureBootSettled
    WaitPowerOffSecureBootSettled --> PowerOffSecureBootSettled: timeout
    WaitPowerOffSecureBootSettled --> AttachMedia: power state off
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
    Cancel --> WaitCancel
    WaitCancel --> Cancel: timeout
    WaitCancel --> Canceled: power state off
    Canceled --> [*]
```

To keep the diagram readable, only some of the edges into `failed` and `cancel`
are drawn. Every state can reach both: a trigger, that exhausts its retry
budget, and a wait, that has no fallback and times out, end in `failed`, and a
cancellation preempts every state but the clean up it triggers itself.

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
survives the wipe is named by the `secure_boot` section of the BIOS profiles,
layered over the allow lists built into Operations Center, which keep the
Microsoft CAs the option ROMs of most hardware are signed with.

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
API.** A deployment for such a server sets `skip_secure_boot_certificates`, which
passes the `secure boot certificates` state by; the certificates then have to be
enrolled by an operator beforehand. The power off keeps its place either way,
since the server has to be off for the installation media to be attached.

### Power and failure handling

**The deployment cuts the power rather than asking for a graceful shutdown.** A
graceful shutdown is an ACPI request, which only an operating system answers,
while a server, that is being deployed, sits in its POST or in the installer: the
BMC accepts the request and the server quietly ignores it.

**A failed deployment is not cleaned up.** The installation media stays attached
and the power state is left as it is, so an operator can look at the server
through the BMC console. A canceled deployment, in contrast, does eject the
media and power the server off.

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
   and records the read progress per image and source address.
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

**Signals 2 and 3 are both gated on the installation being able to be done at
all**, so that neither can end the wait while the installer is still running.
What separates the two cases is the read progress: a firmware, that still has
something to pick up, reboots the server within the first POST cycles of a boot
and keeps the media quiet meanwhile, while the installer streams hundreds of
megabytes of it.

**Signal 2 is therefore carried by the read progress.** A reboot ends the wait as
soon as `ServerDeploymentMediaMinBytesRead` has been read, however long the
installation took — a machine, that is done in five minutes, is not held back for
ten. Only where the read progress can not be told at all does
`ServerDeploymentMinInstallDuration` stand in for it. Where the reboot does not
end the wait, it re-anchors the snapshot instead, so the firmware reboot is not
carried into the next comparison.

**Signal 3 keeps the floor**, since the media going quiet is a weaker statement
than a reboot: the installer stops reading while it partitions the disk, and
ejecting the media then would break the installation. It ends the wait no earlier
than `ServerDeploymentMinInstallDuration` after entering it.

With `force` — a token seed without `force_reboot` — signal 2 does not fire on
its own, since the installer waits for the media to disappear instead of
rebooting. Ejecting the media, which happens right after signal 3, is what makes
IncusOS reboot in that case.

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
amount, alongside `media_size` for the image it is read from.

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

A server, that powered off instead of rebooting, is powered on again on every
tick. A BMC, that reports none of the properties the reboot detection needs, can
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
  counter is incremented and the deployment falls back to the trigger.
* **Call timeout**: independently of both, every attempt of a state runs with a
  deadline of its own, so a BMC, that accepts the connection and then stops
  answering, ends the attempt instead of parking the control loop. Running out
  of it is retryable, so a trigger is issued again and a wait is simply
  evaluated again. Two actions get a budget of their own, since their legitimate
  duration is minutes rather than seconds: **attach media**, which a BMC, that
  uploads the installation media instead of streaming it, only answers once it
  has read the whole image, and **secure boot**, which removes every entry of
  the key databases with a request of its own.
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
