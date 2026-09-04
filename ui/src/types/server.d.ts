export interface Settings {
  api_status: string;
  api_version: number;
  auth?: string;
  auth_methods?: string[];
  server_version: string;
}

export interface ApplicationVersionData {
  name: string;
  version: string;
}

export interface OSVersionData {
  name: string;
  version: string;
  version_next: string;
}

export interface ServerVersionData {
  os: OSVersionData;
  applications: ApplicationVersionData[];
  needs_reboot: boolean;
  needs_update: boolean;
  in_maintenance: number;
}

export interface ServerProperty {
  [key: string]: string;
}

export interface BMCConfig {
  api_type: string;
  endpoint: string;
  certificate: string;
  auto_pin_certificate: boolean;
  username: string;
  password: string;
}

export interface BMCData {
  bmc_protocol: string;
  bmc_protocol_version: string;
  bmc_vendor: string;
  bmc_model: string;
  bmc_firmware_version: string;
  bmc_service_identification: string;
  server_manufacturer: string;
  server_model: string;
  server_sub_model: string;
  system_uuid: string;
  server_asset_tag: string;
  server_host_name: string;
  server_sku: string;
  server_serial_number: string;
  server_bios_version: string;
  server_bios_attributes: Record<string, unknown>;
  server_processor_architecture: string;
  server_processor_instruction_set: string;
  server_power_state: string;
  server_location_indicator_active: boolean;
  server_health_status: string;
  server_last_reset_time: string;
  server_boot_progress: BMCBootProgress;
  virtual_media: Record<string, BMCVirtualMedia>;
  last_updated: string;
}

export interface BMCBootProgress {
  last_state: string;
  last_state_time: string;
  last_boot_time_seconds: number;
  oem_last_state: string;
}

export interface BMCVirtualMedia {
  id: string;
  inserted: boolean;
  image: string;
  image_name: string;
  connected_via: string;
  status: string;
  media_types: string[];
  transfer_method: string;
  transfer_protocol_type: string;
  write_protected: boolean;
}

// BMCLogEvent fields are marshalled without JSON tags, hence the casing.
export interface BMCLogEvent {
  entry_code: string;
  message: string;
  severity: string;
  timestamp: string;
  entry_type: string;
}

export interface ServerDeploymentPost {
  token_uuid: string;
  seed: string;
  type: string;
  architecture: string;
  channel: string;
  virtual_media_id: string;
  force: boolean;
  skip_secure_boot_certificates: boolean;
}

export interface ServerDeploymentStep {
  state: string;
  entered_at: string;
  retries: number;
  error: string;
}

export interface ServerDeploymentStatus {
  state: string;
  request: ServerDeploymentPost;
  force_reboot: boolean;
  bios_profiles: string[];
  bios_attributes: Record<string, unknown>;
  bios_deferred_attributes: Record<string, unknown>;
  media_url: string;
  media_bytes_read: number;
  media_size: number;
  retries: number;
  last_error: string;
  failed_state: string;
  started_at: string;
  state_entered_at: string;
  finished_at: string;
  history: ServerDeploymentStep[];
}

export interface Server {
  name: string;
  cluster: string;
  connection_url: string;
  channel: string;
  description: string;
  properties: ServerProperty;
  public_connection_url: string;
  server_type: string;
  server_status: string;
  server_status_detail: string;
  certificate: string;
  fingerprint: string;
  last_updated: string;
  last_seen: string;
  hardware_data: string;
  os_data: string;
  system_state_is_trusted: boolean;
  version_data: ServerVersionData;
  bmc_config: BMCConfig;
  bmc_data: BMCData;
  deployment?: ServerDeploymentStatus;
}

export interface ServerFormValues {
  name: string;
  public_connection_url: string;
  channel: string;
  description: string;
  properties: ServerProperty;
  network_configuration: string;
  storage_configuration: string;
  bmc_endpoint: string;
  bmc_certificate: string;
  bmc_auto_pin_certificate: boolean;
  bmc_username: string;
  bmc_password: string;
}

export interface ServerPreRegisterFormValues {
  name: string;
  description: string;
  properties: ServerProperty;
  public_connection_url: string;
  channel: string;
  bmc_endpoint: string;
  bmc_certificate: string;
  bmc_auto_pin_certificate: boolean;
  bmc_username: string;
  bmc_password: string;
}
