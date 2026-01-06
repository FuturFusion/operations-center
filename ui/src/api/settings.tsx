import { APIResponse } from "types/response";
import {
  SystemNetwork,
  SystemSecurity,
  SystemSettings,
  SystemUpdates,
} from "types/settings";

export const fetchSystemNetwork = (): Promise<SystemNetwork> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/network`)
      .then((response) => response.json())
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchSystemSecurity = (): Promise<SystemSecurity> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/security`)
      .then((response) => response.json())
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchSystemSettings = (): Promise<SystemSettings> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/settings`)
      .then((response) => response.json())
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchSystemUpdates = (): Promise<SystemUpdates> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/system/updates`)
      .then((response) => response.json())
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
