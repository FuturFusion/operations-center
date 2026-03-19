# Cluster

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
