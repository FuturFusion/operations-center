export interface Token {
  uuid: string;
  description: string;
  expire_at: string;
  uses_remaining: number;
}

export interface TokenFormValues {
  description: string;
  expire_at: string;
  uses_remaining: number;
}

type ImageType = "iso" | "raw";

export interface ApplicationFormValues {
  name: string;
}

export interface ApplicationsFormValues {
  applications: ApplicationFormValues[];
}

export interface InstallTargetFormValues {
  id: string;
}

export interface InstallFormValues {
  force_install: boolean;
  force_reboot: boolean;
  target: InstallTargetFormValues;
}

export interface SeedsFormValues {
  applications: ApplicationsFormValues;
  install: InstallFormValues;
  network: string;
}

export interface DownloadImageFormValues {
  type: ImageType;
  seeds: SeedsFormValues;
}
