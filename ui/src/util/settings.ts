export enum LogLevel {
  Debug = "DEBUG",
  Info = "INFO",
  Warning = "WARN",
  Error = "ERROR",
}

export const ACMEChallengeValues = ["HTTP-01", "DNS-01"] as const;
