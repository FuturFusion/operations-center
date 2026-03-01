export enum ServerType {
  Incus = "incus",
  MigrationManager = "migration-manager",
  OperationsCenter = "operations-center",
}

export const ServerTypeString = {
  "": "",
  incus: "Incus",
  "migration-manager": "Migration Manager",
  "operations-center": "Operations Center",
};

export type ServerTypeKey = keyof typeof ServerTypeString;

export enum ServerAction {
  Evacuate = "evacuate",
  PowerOff = "poweroff",
  Reboot = "reboot",
  Restore = "restore",
  Update = "update",
}
