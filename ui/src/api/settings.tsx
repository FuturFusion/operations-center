import { APIResponse, ErrorMetadata } from "types/response";
import {
  SystemCertificate,
  SystemNetwork,
  SystemSecurity,
  SystemSettings,
  SystemUpdates,
} from "types/settings";
import { APIError, processResponse } from "util/response";

export const fetchSystemCertificate = (): Promise<SystemCertificate> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/certificate`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchSystemNetwork = (): Promise<SystemNetwork> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/network`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchSystemSecurity = (): Promise<SystemSecurity> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/security`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchSystemSettings = (): Promise<SystemSettings> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/settings`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchSystemUpdates = (): Promise<SystemUpdates> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/updates`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const updateSystemCertificate = (
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/certificate`, {
      method: "POST",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const updateSystemNetwork = (
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/network`, {
      method: "PUT",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const updateSystemSecurity = (
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/security`, {
      method: "PUT",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const updateSystemSettings = (
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/settings`, {
      method: "PUT",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const updateSystemUpdates = (
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/updates`, {
      method: "PUT",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const createSystemBackup = (
  withImages: boolean,
  withUpdates: boolean,
): Promise<string> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/:backup`, {
      method: "POST",
      body: JSON.stringify({
        with_images: withImages,
        with_updates: withUpdates,
      }),
      headers: {
        "Content-Type": "application/json",
      },
    })
      .then(async (response) => {
        if (!response.ok) {
          const error =
            (await response.json()) as APIResponse<ErrorMetadata | null>;
          throw new APIError(error);
        }

        return response.blob();
      })
      .then((data) => resolve(URL.createObjectURL(data)))
      .catch(reject);
  });
};

export const restoreSystemBackup = (file: File): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/:restore`, {
      method: "POST",
      body: file,
      headers: {
        "Content-Type": "application/gzip",
      },
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};
