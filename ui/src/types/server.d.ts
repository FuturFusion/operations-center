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

export interface ServerVersionData {
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
  server_processor_architecture: string;
  server_processor_instruction_set: string;
  server_power_state: string;
  server_location_indicator_active: boolean;
  server_health_status: string;
  last_updated: string;
}

// BMCLogEvent fields are marshalled without JSON tags, hence the casing.
export interface BMCLogEvent {
  EntryCode: string;
  Message: string;
  Severity: string;
  Timestamp: string;
  EntryType: string;
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
