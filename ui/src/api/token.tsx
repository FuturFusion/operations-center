import { Token, TokenSeed } from "types/token";
import { APIResponse } from "types/response";
import { processResponse } from "util/response";

export const fetchTokens = (): Promise<Token[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/tokens?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchToken = (uuid: string): Promise<Token> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/tokens/${uuid}`)
      .then((response) => response.json())
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const createToken = (body: string): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/tokens`, {
      method: "POST",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const deleteToken = (uuid: string): Promise<APIResponse<object>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/tokens/${uuid}`, { method: "DELETE" })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const updateToken = (
  uuid: string,
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/tokens/${uuid}`, {
      method: "PUT",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const fetchTokenSeeds = (uuid: string): Promise<TokenSeed[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/tokens/${uuid}/seeds?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchTokenSeed = (
  uuid: string,
  name: string,
): Promise<TokenSeed> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/tokens/${uuid}/seeds/${name}`)
      .then((response) => response.json())
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const createTokenSeed = (
  uuid: string,
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/tokens/${uuid}/seeds`, {
      method: "POST",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const deleteTokenSeed = (
  uuid: string,
  name: string,
): Promise<APIResponse<object>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/tokens/${uuid}/seeds/${name}`, {
      method: "DELETE",
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const updateTokenSeed = (
  uuid: string,
  name: string,
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/tokens/${uuid}/seeds/${name}`, {
      method: "PUT",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const downloadTokenSeedImage = (
  uuid: string,
  name: string,
  type: string,
  arch: string,
): Promise<string> => {
  return new Promise((resolve, reject) => {
    fetch(
      `/1.0/provisioning/tokens/${uuid}/seeds/${name}?type=${type}&architecture=${arch}`,
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

export const downloadImage = (uuid: string, body: string): Promise<string> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/tokens/${uuid}/image`, {
      method: "POST",
      body: body,
    })
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
