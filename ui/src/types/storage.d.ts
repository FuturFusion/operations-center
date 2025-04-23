export interface StorageBucket {
  id: number;
  cluster: string;
  server: string;
  project_name: string;
  parent_name: string;
  name: string;
  object: string;
  last_updated: string;
}

export interface StoragePool {
  id: number;
  cluster: string;
  name: string;
  object: string;
  last_updated: string;
}

export interface StorageVolume {
  id: number;
  cluster: string;
  server: string;
  project_name: string;
  parent_name: string;
  name: string;
  object: string;
  last_updated: string;
}
