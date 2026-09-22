import { Warning } from "types/warning";
import { APIResponse } from "types/response";
import { processResponse } from "util/response";

export const fetchWarnings = (): Promise<Warning[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/warnings`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchWarning = (uuid: string | undefined): Promise<Warning> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/warnings/${uuid}`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const updateWarning = (
  uuid: string | undefined,
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/warnings/${uuid}`, {
      method: "PUT",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};
