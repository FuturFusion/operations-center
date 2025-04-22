import { Instance } from "types/instance";
import { processResponse } from "util/response";

export const fetchInstances = (): Promise<Instance[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/inventory/instances?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};
