import { StorageBucket, StoragePool, StorageVolume } from "types/storage";
import { processResponse } from "util/response";

export const fetchStorageBuckets = (): Promise<StorageBucket[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/inventory/storage_buckets?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchStoragePools = (): Promise<StoragePool[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/inventory/storage_pools?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchStorageVolumes = (): Promise<StorageVolume[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/inventory/storage_volumes?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};
