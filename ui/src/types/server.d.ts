export interface Settings {
  api_status: string;
  api_version: number;
  auth?: string;
  auth_methods?: string[];
}

export interface Server {
  name: string;
  cluster: string;
  connection_url: string;
  channel: string;
  public_connection_url: string;
  server_type: string;
  server_status: string;
  certificate: string;
  fingerprint: string;
  last_updated: string;
  last_seen: string;
  hardware_data: string;
  os_data: string;
  version_data: string;
}

export interface ServerFormValues {
  name: string;
  public_connection_url: string;
  channel: string;
  network_configuration: string;
  storage_configuration: string;
}
