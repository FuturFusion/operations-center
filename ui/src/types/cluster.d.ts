export interface Cluster {
  name: string;
  connection_url: string;
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

export interface ClusterCertFormValues {
  cluster_certificate: string;
  cluster_certificate_key: string;
}
