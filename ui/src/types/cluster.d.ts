export interface Cluster {
  name: string;
  connection_url: string;
  status: string;
  last_updated: string;
}

export interface ClusterCertFormValues {
  cluster_certificate: string;
  cluster_certificate_key: string;
}
