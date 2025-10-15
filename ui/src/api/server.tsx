import { APIResponse } from "types/response";
import { Server, Settings } from "types/server";
import { processResponse } from "util/response";

export const fetchSettings = (): Promise<Settings> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0`)
      .then((response) => response.json())
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchServers = (filter: string): Promise<Server[]> => {
  let url = "/1.0/provisioning/servers?recursion=1";
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

export const fetchServer = (name: string): Promise<Server> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}`)
      .then((response) => response.json())
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const deleteServer = (name: string): Promise<APIResponse<object>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}`, { method: "DELETE" })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const renameServer = (
  name: string,
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}`, {
      method: "POST",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const fetchSystemNetwork = (name: string): Promise<object> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}/system/network`)
      .then((response) => response.json())
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const updateSystemNetwork = (
  name: string,
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}/system/network`, {
      method: "PUT",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};
