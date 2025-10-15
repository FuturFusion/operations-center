import { ClusterTemplate } from "types/cluster_template";
import { APIResponse } from "types/response";
import { processResponse } from "util/response";

export const fetchClusterTemplates = (): Promise<ClusterTemplate[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/cluster-templates?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchClusterTemplate = (
  name: string,
): Promise<ClusterTemplate> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/cluster-templates/${name}`)
      .then((response) => response.json())
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const createClusterTemplate = (
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/cluster-templates`, {
      method: "POST",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const deleteClusterTemplate = (
  name: string,
): Promise<APIResponse<object>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/cluster-templates/${name}`, { method: "DELETE" })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const renameClusterTemplate = (
  name: string,
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/cluster-templates/${name}`, {
      method: "POST",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const updateClusterTemplate = (
  name: string,
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/cluster-templates/${name}`, {
      method: "PUT",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};
