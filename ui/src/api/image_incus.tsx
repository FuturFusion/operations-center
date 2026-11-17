import { APIResponse } from "types/response";
import { IncusImage } from "types/image_incus";
import { localImageSourceName } from "util/image_source";
import { processResponse } from "util/response";

const incusImagesURL = (source: string): string => {
  return `/1.0/images/incus/${encodeURIComponent(source)}/images`;
};

export const fetchIncusImages = (source: string): Promise<IncusImage[]> => {
  return new Promise((resolve, reject) => {
    fetch(`${incusImagesURL(source)}?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

const incusImageURL = (source: string, name: string): string => {
  return `${incusImagesURL(source)}/${encodeURIComponent(name)}`;
};

export const fetchIncusImage = (
  source: string,
  name: string,
): Promise<IncusImage> => {
  return new Promise((resolve, reject) => {
    fetch(incusImageURL(source, name))
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const updateIncusImage = (
  source: string,
  name: string,
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(incusImageURL(source, name), {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const deleteIncusImage = (
  source: string,
  name: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(incusImageURL(source, name), {
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
    fetch(incusImagesURL(localImageSourceName), {
      method: "POST",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

// Upload the metadata tarball together with the image files. The server reads
// the metadata from the first part, which therefore has to be the metadata
// tarball. Its name does not matter, it is recognized by its content.
export const uploadIncusImageFull = (
  metadataFile: File,
  files: File[],
): Promise<APIResponse<null>> => {
  const body = new FormData();
  body.append(metadataFile.name, metadataFile);
  files.forEach((file) => body.append(file.name, file));

  return uploadIncusImage(body);
};

// Upload image files together with the metadata. The server generates the
// metadata tarball from the provided metadata, which therefore has to be the
// first part.
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
  source: string,
  name: string,
  version: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`${incusImageURL(source, name)}/${encodeURIComponent(version)}`, {
      method: "DELETE",
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const downloadIncusImageFile = (
  source: string,
  name: string,
  version: string,
  filename: string,
): Promise<string> => {
  return new Promise((resolve, reject) => {
    fetch(
      `${incusImageURL(source, name)}/${encodeURIComponent(version)}/${encodeURIComponent(filename)}`,
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
