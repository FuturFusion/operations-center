export interface Network {
  id: number;
  cluster: string;
  project_name: string;
  name: string;
  object: string;
  last_updated: string;
}

export interface NetworkACL {
  id: number;
  cluster: string;
  project_name: string;
  name: string;
  object: string;
  last_updated: string;
}

export interface NetworkForward {
  id: number;
  cluster: string;
  parent_name: string;
  name: string;
  object: string;
  last_updated: string;
}

export interface NetworkIntegration {
  id: number;
  cluster: string;
  name: string;
  object: string;
  last_updated: string;
}

export interface NetworkLoadBalancer {
  id: number;
  cluster: string;
  parent_name: string;
  name: string;
  object: string;
  last_updated: string;
}

export interface NetworkPeer {
  id: number;
  cluster: string;
  parent_name: string;
  name: string;
  object: string;
  last_updated: string;
}

export interface NetworkZone {
  id: number;
  cluster: string;
  project_name: string;
  name: string;
  object: string;
  last_updated: string;
}
