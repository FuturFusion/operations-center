# Server

## Server Status Transitions and Update State

The overall state of a server is tracked by a combination of different elements:

Server state:

```{mermaid}
stateDiagram-v2
    state "Unknown" as ServerUnknown
    state "Pending" as ServerPending
    state "Ready" as ServerReady
    state "Offline" as ServerOffline

    ServerUnknown --> ServerPending: create
    ServerPending --> ServerReady: configuration done
    ServerReady --> ServerPending: trigger reconfiguration
    ServerReady --> ServerOffline: reboot|shutdown|unresponsive
    ServerOffline --> ServerReady: online
```

Each server state can have a secondary state (status detail):

* Pending:
   * re-configuring
* Ready:
   * none
   * updating
* Offline:
   * rebooting
   * shut down
   * unresponsive

Each server has a maintenance state:

```{mermaid}
stateDiagram-v2
    state "not in maintenance" as InMaintenanceNotInMaintenance
    state "evacuating" as InMaintenanceEvacuating
    state "evacuated" as InMaintenanceEvacuated
    state "restoring" as InMaintenanceRestoring

    InMaintenanceNotInMaintenance --> InMaintenanceEvacuating: evacuation triggered
    InMaintenanceEvacuating --> InMaintenanceEvacuated: evacuation completed life cycle event received
    InMaintenanceEvacuated --> InMaintenanceRestoring: restore triggered
    InMaintenanceRestoring --> InMaintenanceNotInMaintenance: restore completed life cycle event received
```

The combination of the server state (incl. status detail) and maintenance state
then allows to track the servers update cycle as follows:

```{mermaid}
stateDiagram-v2
    state "Up to date" as UpdateReady
    state "update pending" as UpdatePending
    state "updating" as UpdateUpdating
    state if_requires_reboot <<choice>>
    state if_incus <<choice>>
    state "Evacuation pending" as UpdateNeedsEvacuation
    state "Reboot pending" as UpdateNeedsReboot
    state "Rebooting" as UpdateRebooting
    state "Evacuating" as UpdateEvacuating
    state "In maintenance (reboot pending)" as UpdateInMaintenanceRebootPending
    state "In maintenance (rebooting)" as UpdateInMaintenanceRebooting
    state "In maintenance (restore pending)" as UpdateInMaintenanceRestorePending
    state "Restoring" as UpdateRestoring

    UpdateReady --> UpdatePending: update available
    UpdatePending --> UpdateUpdating: update triggered
    UpdateUpdating --> if_requires_reboot
    if_requires_reboot --> UpdateReady: requires reboot == false
    if_requires_reboot --> if_incus: requires reboot == true
    if_incus --> UpdateNeedsEvacuation: if clusterd Incus
    if_incus --> UpdateNeedsReboot: else
    UpdateNeedsReboot --> UpdateRebooting: trigger reboot
    UpdateRebooting --> UpdateReady: self register or polling

    UpdateNeedsEvacuation --> UpdateEvacuating: evacuation triggered
    UpdateEvacuating --> UpdateInMaintenanceRebootPending: evacuation completed life cycle received
    UpdateInMaintenanceRebootPending --> UpdateInMaintenanceRebooting: trigger reboot
    UpdateInMaintenanceRebooting --> UpdateInMaintenanceRestorePending: self register or polling
    UpdateInMaintenanceRestorePending --> UpdateRestoring: restore triggered
    UpdateRestoring --> UpdateReady: restore completed life cycle event received
```
