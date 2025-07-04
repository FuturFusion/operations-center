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
