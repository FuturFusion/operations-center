import { useQuery } from "@tanstack/react-query";
import { fetchSystemNetwork, updateSystemNetwork } from "api/settings";
import SystemNetworkForm from "components/SystemNetworkForm";
import { useNotification } from "context/notificationContext";
import { SystemNetwork } from "types/settings";

const SystemNetworkConfiguration = () => {
  const { notify } = useNotification();

  const onSubmit = (network: SystemNetwork) => {
    updateSystemNetwork(JSON.stringify(network, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Network settings updated`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during network settings update: ${e}`);
      });
  };

  const {
    data: network = undefined,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["system_network"],
    queryFn: () => fetchSystemNetwork(),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading network settings</div>;
  }

  return <SystemNetworkForm network={network} onSubmit={onSubmit} />;
};

export default SystemNetworkConfiguration;
