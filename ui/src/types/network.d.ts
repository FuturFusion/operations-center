export interface Network {
  uuid: string;
  cluster: string;
  project_name: string;
  name: string;
  object: string;
  last_updated: string;
}

export interface NetworkACL {
  uuid: string;
  cluster: string;
  project_name: string;
  name: string;
  object: string;
  last_updated: string;
}

export interface NetworkForward {
  uuid: string;
  cluster: string;
  parent_name: string;
  name: string;
  object: string;
  last_updated: string;
}

export interface NetworkIntegration {
  uuid: string;
  cluster: string;
  name: string;
  object: string;
  last_updated: string;
}

export interface NetworkLoadBalancer {
  uuid: string;
  cluster: string;
  parent_name: string;
  name: string;
  object: string;
  last_updated: string;
}

export interface NetworkPeer {
  uuid: string;
  cluster: string;
  parent_name: string;
  name: string;
  object: string;
  last_updated: string;
}

export interface NetworkZone {
  uuid: string;
  cluster: string;
  project_name: string;
  name: string;
  object: string;
  last_updated: string;
}
