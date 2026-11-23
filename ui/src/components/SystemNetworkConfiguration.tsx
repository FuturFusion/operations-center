import { useQuery } from "@tanstack/react-query";
import { fetchSystemNetwork, updateSystemNetwork } from "api/settings";
import SystemNetworkForm from "components/SystemNetworkForm";
import { useNotification } from "context/notificationContext";
import { SystemNetwork } from "types/settings";
import { errorMessage } from "util/response";
import QueryError from "components/QueryError";

const SystemNetworkConfiguration = () => {
  const { notify } = useNotification();

  const {
    data: result = undefined,
    error,
    isLoading,
    refetch,
  } = useQuery({
    queryKey: ["system_network"],
    queryFn: () => fetchSystemNetwork(),
  });

  const onSubmit = (network: SystemNetwork) => {
    updateSystemNetwork(JSON.stringify(network, null, 2), result?.etag ?? "")
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Network settings updated`);
          return;
        }
        notify.error(errorMessage(response));
      })
      .catch((e) => {
        notify.error(`Error during network settings update: ${e}`);
      })
      // The ETag changes with every update, after a refused one the current config is shown.
      .finally(() => refetch());
  };

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <QueryError error={error} label="the network settings" />;
  }

  return <SystemNetworkForm network={result?.data} onSubmit={onSubmit} />;
};

export default SystemNetworkConfiguration;
