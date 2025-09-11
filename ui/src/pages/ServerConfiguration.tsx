import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import {
  fetchServer,
  fetchSystemNetwork,
  renameServer,
  updateSystemNetwork,
} from "api/server";
import ServerForm from "components/ServerForm";
import { useNotification } from "context/notificationContext";
import { ServerFormValues } from "types/server";

const ServerConfiguration = () => {
  const { name } = useParams() as { name: string };
  const { notify } = useNotification();
  const navigate = useNavigate();

  const onSubmit = (values: ServerFormValues) => {
    const serverData = { name: values.name };
    const systemNetworkData = {
      network_configuration: values.network_configuration,
    };

    const updateNetworkConfiguration = () => {
      return updateSystemNetwork(
        values.name,
        JSON.stringify(systemNetworkData, null, 2),
      )
        .then((response) => {
          if (response.error_code == 0) {
            notify.success(`Server ${values.name} updated`);
            navigate(`/ui/provisioning/servers/${values.name}/configuration`);
            return;
          }
          notify.error(
            `Error during network configuration update: ${response.error}`,
          );
        })
        .catch((e) => {
          notify.error(`Error during server update: ${e}`);
        });
    };

    if (name !== values.name) {
      renameServer(name, JSON.stringify(serverData, null, 2))
        .then((response) => {
          if (response.error_code == 0) {
            return updateNetworkConfiguration();
          }
          notify.error(response.error);
        })
        .catch((e) => {
          notify.error(`Error during server update: ${e}`);
        });
    } else {
      updateNetworkConfiguration();
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

  if (isServerLoading || isSystemNetworkLoading) {
    return <div>Loading...</div>;
  }

  if (serverError || systemNetworkError) {
    return <div>Error while loading servers</div>;
  }

  return (
    <ServerForm
      server={server}
      systemNetwork={systemNetwork}
      onSubmit={onSubmit}
    />
  );
};

export default ServerConfiguration;
