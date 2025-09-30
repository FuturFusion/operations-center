type ImageType = "iso" | "raw";

type Architecture = "" | "x86_64" | "aarch64";

type YamlValue =
  | string
  | number
  | boolean
  | null
  | YamlValue[]
  | { [key: string]: YamlValue };

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

export interface TokenImageFormValues {
  architecture: Architecture;
  type: ImageType;
  seeds: SeedsFormValues;
}

export interface TokenSeedImageFormValues {
  type: ImageType;
  architecture: Architecture;
}

export interface TokenSeedConfigs {
  applications: YamlValue;
  install: YamlValue;
  network: YamlValue;
}

export interface TokenSeed {
  token_uuid?: string;
  name: string;
  description: string;
  public: boolean;
  seeds: TokenSeedConfigs;
  last_updated?: string;
}

export interface TokenSeedConfigsFormValues {
  applications: string;
  install: string;
  network: string;
}

export interface TokenSeedFormValues {
  name: string;
  description: string;
  public: boolean;
  seeds: TokenSeedConfigsFormValues;
}
