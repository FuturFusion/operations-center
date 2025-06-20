// Code generated by generate-inventory; DO NOT EDIT.

import { StoragePool } from "types/storage_pool";
import { processResponse } from "util/response";

export const fetchStoragePools = (): Promise<StoragePool[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/inventory/storage_pools?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};
