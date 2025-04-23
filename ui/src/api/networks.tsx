import {
  Network,
  NetworkACL,
  NetworkForward,
  NetworkIntegration,
  NetworkLoadBalancer,
  NetworkPeer,
  NetworkZone,
} from "types/network";
import { processResponse } from "util/response";

export const fetchNetworks = (): Promise<Network[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/inventory/networks?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchNetworkACLs = (): Promise<NetworkACL[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/inventory/network_acls?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchNetworkForwards = (): Promise<NetworkForward[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/inventory/network_forwards?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchNetworkIntegrations = (): Promise<NetworkIntegration[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/inventory/network_integrations?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchNetworkLoadBalancers = (): Promise<NetworkLoadBalancer[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/inventory/network_load_balancers?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchNetworkPeers = (): Promise<NetworkPeer[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/inventory/network_peers?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchNetworkZones = (): Promise<NetworkZone[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/inventory/network_zones?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};
