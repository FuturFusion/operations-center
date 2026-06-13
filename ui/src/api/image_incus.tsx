import { APIResponse } from "types/response";
import { IncusImage } from "types/image_incus";
import { processResponse } from "util/response";

export const fetchIncusImages = (): Promise<IncusImage[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/image/incus?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchIncusImage = (name: string): Promise<IncusImage> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/image/incus/${name}`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const updateIncusImage = (
  name: string,
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/image/incus/${name}`, {
      method: "PUT",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const deleteIncusImage = (name: string): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/image/incus/${name}`, {
      method: "DELETE",
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const uploadIncusImageVersion = (
  name: string,
  version: string,
  files: File[],
): Promise<APIResponse<null>> => {
  const body = new FormData();
  files.forEach((file, index) => {
    body.append(`file${String(index).padStart(2, "0")}`, file);
  });

  return new Promise((resolve, reject) => {
    fetch(`/1.0/image/incus/${name}/${version}`, {
      method: "POST",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const deleteIncusImageVersion = (
  name: string,
  version: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/image/incus/${name}/${version}`, {
      method: "DELETE",
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const downloadIncusImageFile = (
  name: string,
  version: string,
  filename: string,
): Promise<string> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/image/incus/${name}/${version}/${filename}`)
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
