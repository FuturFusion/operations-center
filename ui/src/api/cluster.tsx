import { Cluster } from "types/cluster";
import { processResponse } from "util/response";

export const fetchClusters = (): Promise<Cluster[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/clusters?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};
