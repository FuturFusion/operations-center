# Cluster

## Cluster wide operations

A cluster can only ever run a single cluster wide operation at a time. Currently,
two such operations exist:

* `update`, triggered with `POST /1.0/provisioning/clusters/{name}/:update`,
  which performs a rolling update of OS and applications.
* `reboot`, triggered with `POST /1.0/provisioning/clusters/{name}/:reboot`,
  which performs a rolling reboot of all servers.

Whichever operation is currently ongoing is canceled with
`POST /1.0/provisioning/clusters/{name}/:cancel-operation`.

Both operations share the same state machine, which is described in the following
sections.

## Rolling Update

The rolling update process is tracked by a combination of the server state and
the cluster update in progress state and follows the following flow diagram:

```{mermaid}
flowchart TD
    TriggerUpdate[trigger update]@{ shape: circle }
    StatusUpdate[server status event]@{ shape: circle }
    UpdateMonitor[update monitor interval]@{ shape: circle }
    Abort[abort]
    Abort@{ shape: circle }
    UpdateOngoing{update ongoing?}
    RefreshServersTriggerUpdate[refresh all servers]
    ClusterReady{is cluster ready?}
    TriggerAppUpdate[trigger app update]
    AppUpdateRequired{app update required?}
    SetClusterUpdateInProgressStatus[set 'cluster update in progress status']
    TargetStateFromDB[target state from DB]
    OSUpdateDone{OS update done?}
    CleanupDB[cleanup update state from DB]
    End@{ shape: stop }
    NextAction[calculate next action]
    NextActionAllowed{next action allowed?}
    TriggerNextAction[trigger next action, update DB state]
    RefreshServersFromEvent[refresh all servers]
    UpdateOngoingFromEvent{update ongoing?}
    ForEachCluster[for each cluster do]

    TriggerUpdate --> UpdateOngoing
    UpdateOngoing -->|Yes| End
    UpdateOngoing -->|No| RefreshServersTriggerUpdate
    RefreshServersTriggerUpdate --> ClusterReady
    ClusterReady -->|No| End
    ClusterReady -->|Yes| AppUpdateRequired
    AppUpdateRequired -->|No| SetClusterUpdateInProgressStatus
    AppUpdateRequired -->|Yes| TriggerAppUpdate
    TriggerAppUpdate --> AppUpdateRequired
    SetClusterUpdateInProgressStatus --> TargetStateFromDB
    OSUpdateDone -->|Yes| CleanupDB
    CleanupDB --> End
    OSUpdateDone -->|No| NextAction
    NextAction --> NextActionAllowed
    NextActionAllowed -->|No| End
    NextActionAllowed -->|Yes| TriggerNextAction
    TriggerNextAction --> End

    StatusUpdate -----> TargetStateFromDB
    TargetStateFromDB --> UpdateOngoingFromEvent
    UpdateOngoingFromEvent -->|No| End
    UpdateOngoingFromEvent -->|Yes| RefreshServersFromEvent
    RefreshServersFromEvent --> OSUpdateDone

    UpdateMonitor ----> ForEachCluster
    ForEachCluster --> TargetStateFromDB

    Abort -----------> CleanupDB
```

### Tracking the state of a server

What the run has decided about a single server lives in
`Server.StatusInternal.Update`, which is stored as JSON on the server record and
is not part of the REST API surface. It is created for every member of the
cluster when the run is launched and dropped when the run completes or is
aborted.

Two rules make the state machine restart safe:

* **Every step is split into a trigger and a wait.** The trigger is persisted,
  so a retry re-issues it, a wait, that outstays its time, falls back to it, and
  a daemon restart re-enters it.
* **The wait condition is deliberately not persisted.** It is re-derived on
  every tick by `api.Server.UpdateState` from freshly polled data — the Incus
  cluster member status and the version data IncusOS reports. That is the same
  derivation, which reports the progress to the user, so the action taken and
  the progress shown can not disagree.

Both rules rest on the outcome, that Incus reports for an evacuation or a
restore, being an **optimization**. It arrives on a goroutine waiting on the
Incus operation, which dies with the process and never returns if the connection
to the operation is lost. It turns a fast failure into an immediate retry
instead of one, that waits the step timeout out, and nothing more: it never
decides the fate of the run and never touches the cluster.

### Where the state machine is defined

`rollingUpdateStates` in `internal/provisioning/cluster/cluster_update_states.go`
holds what the control loop knows about every update state a server can be
observed in: whether the state triggers a step, waits for one, settles, is
already done or can not be driven at all; which step that is; how long the state
is granted and how many attempts it gets; whether a stalled wait falls back to
issuing the step again; how a server sitting in the state is treated while the
run works on another one; and how a server, that the run leaves evacuated, is
treated in it.

The table covers the rolling restart and the rolling reboot. The update phase is
driven by `executeRollingUpdate`, which reads the polled state directly rather
than the table, so the time an update is granted is only found with the step.

Two things stay outside the table, because they are not properties of a state:
`api.ServerUpdateStateUndefined` and `api.ServerUpdateStateUpdating`, neither of
which belongs to the machine and both of which end the run wherever they are
observed; and the `post_restore_delay` a cluster may configure for itself, which
overrides the delay the table declares.

A state, that is missing from the table, gets the zero value, which ends the run
naming the state.

The steps themselves — the status details Operations Center records while one is
in flight, the time it is granted and the attempts it gets — live next to
`ServerUpdateStep` in `internal/provisioning/server_update_model.go`, since the
server service records them and the cluster service waits on them.

### Retries and timeouts

Every triggered step is granted a time, after which nothing is going to report
its outcome anymore. Within it, the run simply waits. Beyond it, the step is
triggered again, and the attempt is counted, so a step, that keeps stalling,
runs out of attempts and ends the run, naming the server and the step.

The update step covers both of the states, in which a server is updating: the
one of the OS and the one of an application. A rolling update triggers the OS
alone, since an IncusOS update covers the applications as well, but a server can
be left in the application state by an update, which has been requested for it
directly, and the run waits for that one under the same budget.

The reboot is the exception: a server, that does not come back, can not accept
another reboot, so the step has no trigger to fall back to and ends the run
right away.

The same applies to a step, whose record shows no outstanding attempt at all,
which is the state a server is left in by an operation, whose failure has been
reported, and by a status detail, that no longer has an owner.

Launching a run also rewinds a status detail left behind by an earlier one, so
relaunching is a way out of a server, that is stuck in one of the transient
states.

A cluster, which configures no `post_restore_delay`, gets the one the settle
state declares rather than no delay between the restore of a server and the
evacuation of the next.

The two tables name the defaults, which live in
`internal/config/daemon/consts.go`.

## On-demand Rolling Reboot

A rolling reboot reboots every server of a cluster, one at a time, independently
of whether an update is pending. It is meant for reboots, which Operations Center
can not derive from the state reported by IncusOS, e.g. after a firmware update.

It is triggered with `POST /1.0/provisioning/clusters/{name}/:reboot` and, being
a cluster wide operation like the rolling update, is canceled with
`POST /1.0/provisioning/clusters/{name}/:cancel-operation`.

### Reusing the rolling restart

The rolling reboot does not have a control loop of its own. It sets the cluster
update in progress status to `rolling reboot` and is then driven by
`executeRollingRestartNextStep`, the same one-server-at-a-time loop, that performs
the restart phase of a rolling update. It therefore enters the state machine at
`evacuation pending` and skips the two leading update steps, which is why a
rolling reboot has 7 steps per server instead of the 9 steps of a rolling update
with reboot.

### Synthesizing the need for a reboot

The loop is driven entirely by `api.Server.UpdateState`, which derives the state
of a server from `Status`, `StatusDetail` and `VersionData`. With no pending
update, `VersionData.NeedsReboot` is false for every server, so all servers report
`up to date` and the cycle would end immediately.

`ServerUpdate.RebootPending` therefore records, that the server still owes a
reboot, and `serverUpdateStateForRollingUpdate` reports `NeedsReboot = true` for
every server, which does. Pending updates are reported as absent for the whole
run, so a newly published update can not interrupt the cycle, exactly as during
a rolling restart.

A server stops reporting a pending reboot as soon as its reboot has been
triggered.

The rolling update uses the same record, rather than `VersionData.NeedsReboot`,
and for a reason of its own. IncusOS reports the staged version through
`/os/1.0` and the need for a reboot through `/os/1.0/system/update`, and the two
do not flip together: there is a window, in which a server has staged its update
and therefore no longer asks for one, while it does not ask for a reboot yet. A
run, that hands over to the restart phase within that window, sees every server
as `up to date` and finishes without having rebooted anything. The reboot is
therefore recorded as owed the moment the update is triggered, and the run is
seeded at launch with every server, whose staged version is ahead of its running
one.

`ClusterUpdateInProgressStatus.PendingReboot` is still reported over the REST
API, calculated from the records of the servers, the same way `NeedsUpdate`,
`NeedsReboot` and `StatusDescription` are.

### Preconditions and side effects

On top of the checks a rolling update performs, a rolling reboot additionally
requires that no server is busy (`StatusDetail` is empty). This is checked at
launch time, so the user gets to see the reason.

Servers, which have been evacuated manually before the reboot was launched, are
rebooted as well, but are left in the evacuated state afterwards, the same as
during a rolling update.

Rebooting a server applies an IncusOS update, that has already been staged on it.
This is unavoidable, since the staged image is what the server boots.
