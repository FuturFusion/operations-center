export interface Cluster {
  name: string;
  connection_url: string;
  certificate: string;
  fingerprint: string;
  status: string;
  last_updated: string;
}

export interface ClusterPost {
  name: string;
  connection_url: string;
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
}

export interface ClusterCertFormValues {
  cluster_certificate: string;
  cluster_certificate_key: string;
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
