export interface StorageBucket {
  uuid: string;
  cluster: string;
  server: string;
  project_name: string;
  parent_name: string;
  name: string;
  object: string;
  last_updated: string;
}

export interface StoragePool {
  uuid: string;
  cluster: string;
  name: string;
  object: string;
  last_updated: string;
}

export interface StorageVolume {
  uuid: string;
  cluster: string;
  server: string;
  project_name: string;
  parent_name: string;
  name: string;
  object: string;
  last_updated: string;
}
