import { APIResponse } from "types/response";
import type {
  IncusOSApplication,
  IncusOSLog,
  IncusOSSettings,
  IncusOSSystemUpdate,
} from "types/os";
import { processResponse } from "util/response";

export const isIncusOS = async (): Promise<boolean> => {
  return new Promise((resolve, reject) => {
    fetch("/os/1.0")
      .then((response) => response.json())
      .then((data) => {
        if (data.error_code == 0) {
          return resolve(true);
        }
        return resolve(false);
      })
      .catch(reject);
  });
};

export const fetchOS = async (): Promise<IncusOSSettings> => {
  return new Promise((resolve, reject) => {
    fetch("/os/1.0")
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchOSApplications = async (): Promise<string[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/os/1.0/applications`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchOSApplication = async (
  name: string,
): Promise<IncusOSApplication> => {
  return new Promise((resolve, reject) => {
    fetch(name)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchSystemUpdate = async (): Promise<IncusOSSystemUpdate> => {
  return new Promise((resolve, reject) => {
    fetch(`/os/1.0/system/update`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchDebugLogs = async (limit: number): Promise<IncusOSLog[]> => {
  const url = new URL("/os/1.0/debug/log", window.location.origin);

  if (limit > 0) {
    url.searchParams.set("entries", limit.toString());
  }

  return new Promise((resolve, reject) => {
    fetch(url.pathname + url.search)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchOSNetwork = async (): Promise<string> => {
  return new Promise((resolve, reject) => {
    fetch(`/os/1.0/system/network`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const updateOSNetwork = async (
  network: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch("/os/1.0/system/network", {
      method: "PUT",
      body: network,
      headers: {
        "Content-Type": "application/json",
      },
    })
      .then(processResponse)
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const fetchOSStorage = async (): Promise<string> => {
  return new Promise((resolve, reject) => {
    fetch(`/os/1.0/system/storage`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const updateOSStorage = async (
  storage: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch("/os/1.0/system/storage", {
      method: "PUT",
      body: storage,
      headers: {
        "Content-Type": "application/json",
      },
    })
      .then(processResponse)
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const fetchOSSecurity = async (): Promise<string> => {
  return new Promise((resolve, reject) => {
    fetch(`/os/1.0/system/security`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const updateOSSecurity = async (
  security: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch("/os/1.0/system/security", {
      method: "PUT",
      body: security,
      headers: {
        "Content-Type": "application/json",
      },
    })
      .then(processResponse)
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const fetchOSServices = async (): Promise<string[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/os/1.0/services`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchOSService = async (name: string): Promise<string> => {
  return new Promise((resolve, reject) => {
    fetch(`/os/1.0/services/${name}`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const updateOSService = async (
  name: string,
  config: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/os/1.0/services/${name}`, {
      method: "PUT",
      body: config,
      headers: {
        "Content-Type": "application/json",
      },
    })
      .then(processResponse)
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const updateCheck = async (): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch("/os/1.0/system/update/:check", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
    })
      .then(processResponse)
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const poweroffOS = async (): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch("/os/1.0/system/:poweroff", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
    })
      .then(processResponse)
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const rebootOS = async (): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch("/os/1.0/system/:reboot", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
    })
      .then(processResponse)
      .then((data) => resolve(data))
      .catch(reject);
  });
};
