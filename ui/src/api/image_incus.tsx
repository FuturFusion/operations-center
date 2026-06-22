import { APIResponse } from "types/response";
import { IncusImage } from "types/image_incus";
import { processResponse } from "util/response";

export const fetchIncusImages = (): Promise<IncusImage[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/images/incus?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchIncusImage = (name: string): Promise<IncusImage> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/images/incus/${name}`)
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
    fetch(`/1.0/images/incus/${name}`, {
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
    fetch(`/1.0/images/incus/${name}`, {
      method: "DELETE",
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export interface IncusImageMetadata {
  os: string;
  release: string;
  arch: string;
  variant: string;
  version: string;
}

const uploadIncusImage = (body: FormData): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/images/incus`, {
      method: "POST",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

// Upload a complete set of image files including incus.tar.xz. The server reads
// the metadata from incus.tar.xz, which therefore has to be the first part.
export const uploadIncusImageFull = (
  files: File[],
): Promise<APIResponse<null>> => {
  const body = new FormData();
  const incusTarXZ = files.filter((file) => file.name == "incus.tar.xz");
  const others = files.filter((file) => file.name != "incus.tar.xz");
  [...incusTarXZ, ...others].forEach((file) => body.append(file.name, file));

  return uploadIncusImage(body);
};

// Upload image files together with the metadata. The server generates the
// incus.tar.xz from the provided metadata, which therefore has to be the first
// part.
export const uploadIncusImageWithMetadata = (
  metadata: IncusImageMetadata,
  files: File[],
): Promise<APIResponse<null>> => {
  const body = new FormData();
  body.append("request_json", JSON.stringify(metadata));
  files.forEach((file) => body.append(file.name, file));

  return uploadIncusImage(body);
};

export const deleteIncusImageVersion = (
  name: string,
  version: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/images/incus/${name}/${version}`, {
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
    fetch(`/1.0/images/incus/${name}/${version}/${filename}`)
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
