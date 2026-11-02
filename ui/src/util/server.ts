import type { Server } from "types/server";

export enum ServerType {
  Incus = "incus",
  IncusLTS70 = "incus-lts-7.0",
  MigrationManager = "migration-manager",
  OperationsCenter = "operations-center",
}

export const ServerTypeString = {
  "": "",
  incus: "Incus",
  "incus-lts-7.0": "Incus (7.0 LTS)",
  "migration-manager": "Migration Manager",
  "operations-center": "Operations Center",
};

export const IncusServerTypeString = {
  incus: "Incus",
  "incus-lts-7.0": "Incus (7.0 LTS)",
};

export type ServerTypeKey = keyof typeof ServerTypeString;

// Returns the version of the primary application, preferring the friendly one.
export const primaryApplicationVersion = (server: Server): string => {
  const application = server.version_data?.applications?.find(
    (app) => app.name !== "" && app.name in ServerTypeString,
  );

  return application?.friendly_version || application?.version || "";
};

export enum ServerStatus {
  Unknown = "unknown",
  Unregistered = "unregistered",
  Deploying = "deploying",
  Pending = "pending",
  Ready = "ready",
  Offline = "offline",
}

export enum ServerAction {
  Evacuate = "evacuate",
  PowerOff = "poweroff",
  Reboot = "reboot",
  Restore = "restore",
  Update = "update",
}
