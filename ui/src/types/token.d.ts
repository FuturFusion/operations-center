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

export interface InstallTargetFormValues {
  id: string;
}

export interface InstallFormValues {
  force_install: boolean;
  force_reboot: boolean;
  target: InstallTargetFormValues;
}

export interface SeedsFormValues {
  application: string;
  secondary_applications: string[];
  install: InstallFormValues;
  network: string;
  migration_manager: string;
  operations_center: string;
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

export interface TokenSeedApplication {
  name: string;
}

export interface TokenSeedApplications {
  applications: TokenSeedApplication[];
}

export interface TokenSeedConfigs {
  applications: TokenSeedApplications;
  install: YamlValue;
  network: YamlValue;
  migration_manager: YamlValue;
  operations_center: YamlValue;
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
  application: string;
  secondary_applications: string[];
  install: string;
  network: string;
  migration_manager: string;
  operations_center: string;
}

export interface TokenSeedFormValues {
  name: string;
  description: string;
  public: boolean;
  seeds: TokenSeedConfigsFormValues;
}
