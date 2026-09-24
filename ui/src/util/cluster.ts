export const RestoreModeValues = {
  "": "Bring back the instances",
  skip: "Only bring back the server",
} as const;

export const ClusterUpdateInProgress = {
  Inactive: "",
  ApplyUpdate: "applying updates",
  ApplyUpdateWithReboot: "applying updates with reboot",
  RollingRestart: "restarting servers",
  RollingReboot: "rolling reboot",
  Error: "error",
} as const;

export const validateConnectionURL = (value: string): string | undefined => {
  if (!value.trim()) {
    return "Connection URL is required";
  }

  return undefined;
};
