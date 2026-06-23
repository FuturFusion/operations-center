import { APIResponse } from "types/response";
import type {
  IncusOSApplication,
  IncusOSConfig,
  IncusOSLog,
  IncusOSSettings,
  IncusOSSystemUpdate,
} from "types/os";
import { processResponse } from "util/response";

export interface DebugLogOptions {
  unit?: string;
  boot?: string;
  entries?: number;
}

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

export const fetchDebugLogs = async (
  options: DebugLogOptions,
): Promise<IncusOSLog[]> => {
  const url = new URL("/os/1.0/debug/log", window.location.origin);

  if (options.entries && options.entries > 0) {
    url.searchParams.set("entries", options.entries.toString());
  }

  if (options.unit) {
    url.searchParams.set("unit", options.unit);
  }

  if (options.boot) {
    url.searchParams.set("boot", options.boot);
  }

  return new Promise((resolve, reject) => {
    fetch(url.pathname + url.search)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchDebugProcesses = async (): Promise<string> => {
  return new Promise((resolve, reject) => {
    fetch("/os/1.0/debug/processes")
      .then(processResponse)
      .then((data) => resolve(data.metadata))
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

// Generic helpers shared by the OS pages.

// Fetch a config section (returns the {state, config} object).
export const fetchOSSection = async (
  endpoint: string,
): Promise<IncusOSConfig> => {
  return new Promise((resolve, reject) => {
    fetch(`/os/1.0/${endpoint}`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

// Update the config of a section.
export const updateOSSection = async (
  endpoint: string,
  config: object,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/os/1.0/${endpoint}`, {
      method: "PUT",
      body: JSON.stringify({ config: config }),
      headers: {
        "Content-Type": "application/json",
      },
    })
      .then(processResponse)
      .then((data) => resolve(data))
      .catch(reject);
  });
};

// Run an action (POST /os/1.0/<endpoint>/:<action>).
export const runOSAction = async (
  endpoint: string,
  action: string,
  data?: object,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/os/1.0/${endpoint}/:${action}`, {
      method: "POST",
      body: data === undefined ? undefined : JSON.stringify(data),
      headers: {
        "Content-Type": "application/json",
      },
    })
      .then(processResponse)
      .then((data) => resolve(data))
      .catch(reject);
  });
};

// Run an action returning a file, resolving to an object URL.
export const runOSActionDownload = async (
  endpoint: string,
  action: string,
  data?: object,
): Promise<string> => {
  return new Promise((resolve, reject) => {
    fetch(`/os/1.0/${endpoint}/:${action}`, {
      method: "POST",
      body: data === undefined ? undefined : JSON.stringify(data),
      headers: {
        "Content-Type": "application/json",
      },
    })
      .then(async (response) => {
        if (!response.ok) {
          const r = await response.json();
          throw Error(r.error);
        }

        return response.blob();
      })
      .then((blob) => resolve(URL.createObjectURL(blob)))
      .catch(reject);
  });
};

// Run an action taking a file as input.
export const runOSActionUpload = async (
  endpoint: string,
  action: string,
  file: File,
  query?: Record<string, string>,
): Promise<APIResponse<null>> => {
  const url = new URL(`/os/1.0/${endpoint}/:${action}`, window.location.origin);

  for (const [key, value] of Object.entries(query ?? {})) {
    if (value) {
      url.searchParams.set(key, value);
    }
  }

  return new Promise((resolve, reject) => {
    fetch(url.pathname + url.search, {
      method: "POST",
      body: file,
      headers: {
        "Content-Type": "application/octet-stream",
      },
    })
      .then(processResponse)
      .then((data) => resolve(data))
      .catch(reject);
  });
};

// Add a new application.
export const addOSApplication = async (
  name: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch("/os/1.0/applications", {
      method: "POST",
      body: JSON.stringify({ name: name }),
      headers: {
        "Content-Type": "application/json",
      },
    })
      .then(processResponse)
      .then((data) => resolve(data))
      .catch(reject);
  });
};
