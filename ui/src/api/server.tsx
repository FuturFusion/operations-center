import { APIResponse } from "types/response";
import { BMCLogEvent, Server, Settings } from "types/server";
import { Changelog } from "types/changelog";
import { processResponse } from "util/response";

export const fetchSettings = (): Promise<Settings> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0`)
      .then((response) => response.json())
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchServers = (filter: string): Promise<Server[]> => {
  let url = "/1.0/provisioning/servers?recursion=1";
  if (filter) {
    url += `&filter=${filter}`;
  }

  return new Promise((resolve, reject) => {
    fetch(url)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchServer = (name: string): Promise<Server> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}`)
      .then((response) => response.json())
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchServerChangelog = (name: string): Promise<Changelog> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}/changelog`)
      .then((response) => response.json())
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const preRegisterServer = (body: string): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers`, {
      method: "POST",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const deployServer = (name: string, body: string): Promise<void> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}/:deploy`, {
      method: "POST",
      body: body,
    })
      .then(processResponse)
      .then(() => resolve())
      .catch(reject);
  });
};

export const cancelServerDeployment = (name: string): Promise<void> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}/:cancel-deploy`, {
      method: "POST",
    })
      .then(processResponse)
      .then(() => resolve())
      .catch(reject);
  });
};

export const deleteServer = (name: string): Promise<APIResponse<object>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}`, { method: "DELETE" })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const renameServer = (
  name: string,
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}`, {
      method: "POST",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const updateServer = (
  name: string,
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}`, {
      method: "PUT",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const resyncServer = (name: string): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}/:resync`, {
      method: "POST",
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const evacuateServer = (name: string): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}/system/:evacuate`, {
      method: "POST",
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const poweroffServer = (name: string): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}/system/:poweroff`, {
      method: "POST",
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const rebootServer = (name: string): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}/system/:reboot`, {
      method: "POST",
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const restoreServer = (name: string): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}/system/:restore`, {
      method: "POST",
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const updateSystemServer = (
  name: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}/system/:update`, {
      method: "POST",
      // TODO: Hardcoded because the server currently supports only OS updates.
      body: JSON.stringify({ os: { trigger_update: true } }),
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const refreshServerBMC = (name: string): Promise<void> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}/bmc/:refresh`, {
      method: "POST",
    })
      .then(processResponse)
      .then(() => resolve())
      .catch(reject);
  });
};

const serverBMCPowerAction = (
  name: string,
  action: string,
  force: boolean,
): Promise<void> => {
  return new Promise((resolve, reject) => {
    fetch(
      `/1.0/provisioning/servers/${name}/bmc/:${action}${force ? "?force=1" : ""}`,
      {
        method: "POST",
      },
    )
      .then(processResponse)
      .then(() => resolve())
      .catch(reject);
  });
};

export const powerOnServerBMC = (
  name: string,
  force: boolean,
): Promise<void> => {
  return serverBMCPowerAction(name, "server-power-on", force);
};

export const powerOffServerBMC = (
  name: string,
  force: boolean,
): Promise<void> => {
  return serverBMCPowerAction(name, "server-power-off", force);
};

export const restartServerBMC = (
  name: string,
  force: boolean,
): Promise<void> => {
  return serverBMCPowerAction(name, "server-restart", force);
};

export const ServerSetLocationIndicatorBMC = (
  name: string,
  active: boolean,
): Promise<void> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}/bmc/:server-locate`, {
      method: "POST",
      body: JSON.stringify({ active: active }),
    })
      .then(processResponse)
      .then(() => resolve())
      .catch(reject);
  });
};

export const fetchServerBMCLogSources = (name: string): Promise<string[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}/bmc/logs`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchServerBMCLogEntries = (
  name: string,
  logSource: string,
): Promise<BMCLogEvent[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}/bmc/logs/${logSource}`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const fetchSystemNetwork = (name: string): Promise<object> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}/system/network`)
      .then((response) => response.json())
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const updateSystemNetwork = (
  name: string,
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}/system/network`, {
      method: "PUT",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};

export const fetchSystemStorage = (name: string): Promise<object> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}/system/storage`)
      .then((response) => response.json())
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};

export const updateSystemStorage = (
  name: string,
  body: string,
): Promise<APIResponse<null>> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/provisioning/servers/${name}/system/storage`, {
      method: "PUT",
      body: body,
    })
      .then((response) => response.json())
      .then((data) => resolve(data))
      .catch(reject);
  });
};
