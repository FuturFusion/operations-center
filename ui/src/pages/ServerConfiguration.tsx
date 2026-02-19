import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import {
  fetchServer,
  fetchSystemNetwork,
  fetchSystemStorage,
  renameServer,
  updateServer,
  updateSystemNetwork,
  updateSystemStorage,
} from "api/server";
import ServerForm from "components/ServerForm";
import { useNotification } from "context/notificationContext";
import { APIResponse } from "types/response";
import { ServerFormValues } from "types/server";
import YAML from "yaml";

const ServerConfiguration = () => {
  const { name } = useParams() as { name: string };
  const { notify } = useNotification();
  const navigate = useNavigate();

  const onSubmit = async (
    values: ServerFormValues,
  ): Promise<APIResponse<null> | void> => {
    let networkConfig = {};
    try {
      networkConfig = YAML.parse(values.network_configuration);
    } catch (error) {
      notify.error(`Error during YAML network value parsing: ${error}`);
      return;
    }

    let storageConfig = {};
    try {
      storageConfig = YAML.parse(values.storage_configuration);
    } catch (error) {
      notify.error(`Error during YAML storage value parsing: ${error}`);
      return;
    }

    const networkUpdateSuccess = await updateSystemNetwork(
      values.name,
      JSON.stringify(networkConfig, null, 2),
    )
      .then((response) => {
        if (response.error_code == 0) {
          return true;
        }

        notify.error(
          `Error during network configuration update: ${response.error}`,
        );
        return false;
      })
      .catch((e) => {
        notify.error(`Error during server network update: ${e}`);
        return false;
      });

    if (!networkUpdateSuccess) {
      return;
    }

    const storageUpdateSuccess = await updateSystemStorage(
      values.name,
      JSON.stringify(storageConfig, null, 2),
    )
      .then((response) => {
        if (response.error_code == 0) {
          return true;
        }

        notify.error(
          `Error during storage configuration update: ${response.error}`,
        );
        return false;
      })
      .catch((e) => {
        notify.error(`Error during server storage update: ${e}`);
        return false;
      });

    if (!storageUpdateSuccess) {
      return;
    }

    return updateServer(
      values.name,
      JSON.stringify(
        {
          public_connection_url: values.public_connection_url,
          channel: values.channel,
        },
        null,
        2,
      ),
    )
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Server ${values.name} updated`);
          return;
        }
        notify.error(`Error during server update: ${response.error}`);
      })
      .catch((e) => {
        notify.error(`Error during server update: ${e}`);
      });
  };

  const onRename = (newName: string) => {
    if (name !== newName) {
      renameServer(name, JSON.stringify({ name: newName }, null, 2))
        .then((response) => {
          if (response.error_code == 0) {
            notify.success(`Server ${newName} renamed`);
            navigate(`/ui/provisioning/servers/${newName}/configuration`);
            return;
          }
          notify.error(response.error);
        })
        .catch((e) => {
          notify.error(`Error during server rename: ${e}`);
        });
    }
  };

  const {
    data: server = undefined,
    error: serverError,
    isLoading: isServerLoading,
  } = useQuery({
    queryKey: ["servers", name],
    queryFn: () => fetchServer(name),
  });

  const {
    data: systemNetwork = undefined,
    error: systemNetworkError,
    isLoading: isSystemNetworkLoading,
  } = useQuery({
    queryKey: ["servers", name, "system-network"],
    queryFn: () => fetchSystemNetwork(name),
  });

  const {
    data: systemStorage = undefined,
    error: systemStorageError,
    isLoading: isSystemStorageLoading,
  } = useQuery({
    queryKey: ["servers", name, "system-storage"],
    queryFn: () => fetchSystemStorage(name),
  });

  if (isServerLoading || isSystemNetworkLoading || isSystemStorageLoading) {
    return <div>Loading...</div>;
  }

  if (serverError || systemNetworkError || systemStorageError) {
    return <div>Error while loading servers</div>;
  }

  return (
    <ServerForm
      server={server}
      systemNetwork={systemNetwork}
      systemStorage={systemStorage}
      onRename={onRename}
      onSubmit={onSubmit}
    />
  );
};

export default ServerConfiguration;
