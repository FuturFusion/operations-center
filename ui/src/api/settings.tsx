import { APIResponse } from "types/response";
import {
  SystemCertificate,
  SystemNetwork,
  SystemSecurity,
  SystemSettings,
  SystemUpdates,
} from "types/settings";
import { processResponse } from "util/response";

// WithETag holds a config and the ETag of its part shared by the cluster members.
export interface WithETag<T> {
  data: T;
  etag: string;
}

const fetchWithETag = async <T,>(url: string): Promise<WithETag<T>> => {
  const response = await fetch(url);
  const etag = response.headers.get("ETag") ?? "";
  const data = await processResponse(response);

  return { data: data.metadata as T, etag: etag };
};

// ifMatch returns the header, which refuses an update if the config changed since it was fetched.
const ifMatch = (etag: string): Record<string, string> => {
  return etag ? { "If-Match": etag } : {};
};

export const fetchSystemCertificate = (): Promise<SystemCertificate> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/certificate`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchSystemNetwork = (): Promise<WithETag<SystemNetwork>> => {
  return fetchWithETag(`/1.0/system/network`);
};

export const fetchSystemSecurity = (): Promise<WithETag<SystemSecurity>> => {
  return fetchWithETag(`/1.0/system/security`);
};

export const fetchSystemSettings = (): Promise<WithETag<SystemSettings>> => {
  return fetchWithETag(`/1.0/system/settings`);
};

export const fetchSystemUpdates = (): Promise<WithETag<SystemUpdates>> => {
  return fetchWithETag(`/1.0/system/updates`);
};

export const updateSystemCertificate = (
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/certificate`, {
      method: "POST",
      body: body,
      headers: {
        "Content-Type": "application/json",
      },
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const updateSystemNetwork = (
  body: string,
  etag: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/network`, {
      method: "PUT",
      body: body,
      headers: {
        "Content-Type": "application/json",
        ...ifMatch(etag),
      },
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const updateSystemSecurity = (
  body: string,
  etag: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/security`, {
      method: "PUT",
      body: body,
      headers: {
        "Content-Type": "application/json",
        ...ifMatch(etag),
      },
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const updateSystemSettings = (
  body: string,
  etag: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/settings`, {
      method: "PUT",
      body: body,
      headers: {
        "Content-Type": "application/json",
        ...ifMatch(etag),
      },
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const updateSystemUpdates = (
  body: string,
  etag: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/updates`, {
      method: "PUT",
      body: body,
      headers: {
        "Content-Type": "application/json",
        ...ifMatch(etag),
      },
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};
