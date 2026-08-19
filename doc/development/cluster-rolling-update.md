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

`ClusterUpdateInProgressStatus.PendingReboot` therefore holds the list of servers,
which still have to be rebooted. It is populated with all servers of the cluster
when the reboot is launched, and `serverUpdateStateForRollingUpdate` reports
`NeedsReboot = true` for every server on that list. Pending updates are reported
as absent for the whole run, so a newly published update can not interrupt the
cycle, exactly as during a rolling restart.

A server is removed from `PendingReboot` as soon as its reboot has been triggered.

### Preconditions and side effects

On top of the checks a rolling update performs, a rolling reboot additionally
requires that no server is busy (`StatusDetail` is empty). This is checked at
launch time, so the user gets to see the reason.

Servers, which have been evacuated manually before the reboot was launched, are
rebooted as well, but are left in the evacuated state afterwards, the same as
during a rolling update.

Rebooting a server applies an IncusOS update, that has already been staged on it.
This is unavoidable, since the staged image is what the server boots.
