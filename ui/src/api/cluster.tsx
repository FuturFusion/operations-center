import { Cluster } from "types/cluster";
import { APIResponse } from "types/response";
import { processResponse } from "util/response";

export const fetchClusters = (): Promise<Cluster[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/clusters?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const createCluster = (body: string): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/clusters`, {
      method: "POST",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const updateClusterCert = (
  name: string,
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/clusters/${name}/certificate`, {
      method: "PUT",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const downloadTerraformData = (name: string): Promise<string> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/clusters/${name}/terraform-configuration`)
      .then(async (response) => {
        if (!response.ok) {
          const r = await response.json();
          throw Error(r.error);
        }

        return response.blob();
      })
      .then((data) => resolve(URL.createObjectURL(data)))
      .catch(reject);
  });
};
