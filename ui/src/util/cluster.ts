export const RestoreModeValues = {
  "": "Bring back the instances",
  skip: "Only bring back the server",
} as const;

export const ClusterUpdateInProgress = {
  Inactive: "",
  ApplyUpdate: "applying updates",
  ApplyUpdateWithReboot: "applying updates with reboot",
  RollingRestart: "restarting servers",
  Error: "error",
} as const;
