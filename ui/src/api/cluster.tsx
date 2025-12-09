import { Cluster, ClusterArtifact } from "types/cluster";
import { APIResponse } from "types/response";
import { processResponse } from "util/response";

export const fetchClusters = (filter: string): Promise<Cluster[]> => {
  let url = "/1.0/provisioning/clusters?recursion=1";
  if (filter) {
    url += `&filter=${filter}`;
  }

  return new Promise((resolve, reject) => {
    fetch(url)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchCluster = (name: string): Promise<Cluster> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/clusters/${name}`)
      .then((response) => response.json())
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchClusterArtifacts = (
  name: string,
): Promise<ClusterArtifact[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/clusters/${name}/artifacts?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchClusterArtifact = (
  clusterName: string,
  artifactName: string,
): Promise<ClusterArtifact> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/clusters/${clusterName}/artifacts/${artifactName}`)
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

export const deleteCluster = (
  name: string,
  mode: string,
): Promise<APIResponse<object>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/clusters/${name}?mode=${mode}`, {
      method: "DELETE",
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const resyncClusterInventory = (
  name: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/clusters/${name}/resync-inventory`, {
      method: "POST",
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

export const downloadArtifact = (
  clusterName: string,
  artifactName: string,
): Promise<string> => {
  return new Promise((resolve, reject) => {
    fetch(
      `/1.0/provisioning/clusters/${clusterName}/artifacts/${artifactName}?archive=zip`,
    )
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

export const downloadArtifactFile = (
  clusterName: string,
  artifactName: string,
  filename: string,
): Promise<string> => {
  return new Promise((resolve, reject) => {
    fetch(
      `/1.0/provisioning/clusters/${clusterName}/artifacts/${artifactName}/${filename}`,
    )
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
