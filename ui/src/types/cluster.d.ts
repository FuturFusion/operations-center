export interface ClusterUpdateStatusProgress {
  in_progress: string;
  status_description: string;
}

export interface ClusterUpdateStatus {
  needs_reboot: string[];
  needs_update: string[];
  in_maintenance: string[];
  in_progress_status: ClusterUpdateStatusProgress;
}

export interface ClusterProperty {
  [key: string]: string;
}

export interface ClusterConfigRollingRestart {
  post_restore_delay: string;
  restore_mode: string;
}

export interface ClusterConfig {
  rolling_restart: ClusterConfigRollingRestart;
}

export interface Cluster {
  name: string;
  connection_url: string;
  channel: string;
  description: string;
  properties: ClusterProperty;
  config: ClusterConfig;
  certificate: string;
  fingerprint: string;
  status: string;
  last_updated: string;
  update_status: ClusterUpdateStatus;
}

export interface ClusterPost {
  name: string;
  connection_url: string;
  channel: string;
  description: string;
  properties: ClusterProperty;
  config: ClusterConfig;
  server_names: string[];
  server_type: string;
  services_config: YamlValue;
  application_seed_config: YamlValue;
  cluster_template: string;
  cluster_template_variable_values: YamlValue;
}

export interface ClusterFormValues {
  name: string;
  connection_url: string;
  channel: string;
  description: string;
  properties: ClusterProperty;
  restore_mode: string;
  post_restore_delay: string;
}

export interface ClusterCertFormValues {
  cluster_certificate: string;
  cluster_certificate_key: string;
}

export interface ClusterBulkUpdateFormValues {
  action: string;
  arguments: YamlValue;
}

export interface ClusterArtifactFile {
  name: string;
  mime_type: string;
  size: number;
}

export interface ClusterArtifact {
  name: string;
  description: string;
  properties: YamlValue;
  files: ClusterArtifactFile[];
  last_updated: string;
}
