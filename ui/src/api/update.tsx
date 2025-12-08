import { APIResponse } from "types/response";
import { Update, UpdateFile } from "types/update";
import { processResponse } from "util/response";

export const fetchUpdates = (): Promise<Update[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/updates?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchUpdate = (uuid: string): Promise<Update> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/updates/${uuid}`)
      .then((response) => response.json())
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchUpdateFiles = (uuid: string): Promise<UpdateFile[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/updates/${uuid}/files`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const downloadUpdateFile = (
  uuid: string,
  filename: string,
): Promise<string> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/updates/${uuid}/files/${filename}`)
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

export const refreshUpdates = (): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/updates/:refresh`, {
      method: "POST",
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const cleanupUpdates = (): Promise<APIResponse<object>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/updates`, {
      method: "DELETE",
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};
