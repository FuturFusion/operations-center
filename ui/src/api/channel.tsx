import { Channel } from "types/channel";
import { APIResponse } from "types/response";
import { processResponse } from "util/response";

export const fetchChannels = (): Promise<Channel[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/channels?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchChannel = (name: string): Promise<Channel> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/channels/${name}`)
      .then((response) => response.json())
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const createChannel = (body: string): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/channels`, {
      method: "POST",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const deleteChannel = (name: string): Promise<APIResponse<object>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/channels/${name}`, { method: "DELETE" })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const updateChannel = (
  name: string,
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/channels/${name}`, {
      method: "PUT",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};
