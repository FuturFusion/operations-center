import { APIResponse } from "types/response";
import { ImageSource } from "types/image_incus_source";
import { processResponse } from "util/response";

export const fetchImageSources = (): Promise<ImageSource[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/images/incus/sources?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchImageSource = (name: string): Promise<ImageSource> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/images/incus/sources/${name}`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const createImageSource = (body: string): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/images/incus/sources`, {
      method: "POST",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const updateImageSource = (
  name: string,
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/images/incus/sources/${name}`, {
      method: "PUT",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const deleteImageSource = (name: string): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/images/incus/sources/${name}`, {
      method: "DELETE",
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const refreshImageSource = (
  name: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/images/incus/sources/${name}/:refresh`, {
      method: "POST",
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};
