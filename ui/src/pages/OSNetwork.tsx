import type { FC } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { fetchOSNetwork, updateOSNetwork } from "api/os";
import YAMLEditor from "components/YAMLEditor";
import { useNotification } from "context/notificationContext";
import type { IncusOSConfig } from "types/os";
import YAML from "yaml";

const OSNetwork: FC = () => {
  const queryClient = useQueryClient();
  const { notify } = useNotification();

  const {
    data: networkData = "",
    isLoading,
    error,
  } = useQuery({
    queryKey: ["os-network"],
    queryFn: async () => fetchOSNetwork(),
  });

  const updateNetwork = (value: string): Promise<boolean> => {
    let networkConfig = {} as IncusOSConfig;

    try {
      networkConfig = YAML.parse(value);
    } catch (error) {
      notify.error(`Error during YAML value parsing: ${error}`);
      return Promise.resolve(false);
    }

    return updateOSNetwork(JSON.stringify({ config: networkConfig.config }))
      .then(() => {
        notify.success(`Network updated`);
        queryClient.invalidateQueries({ queryKey: ["os-network"] });
        return true;
      })
      .catch((e) => {
        notify.error(`Network update failed: ${e}`);
        return false;
      });
  };

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading network data</div>;
  }

  return (
    <div className="d-flex" style={{ height: "70vh" }}>
      <div className="flex-grow-1">
        <YAMLEditor
          yamlData={YAML.stringify(networkData, null, 2)}
          onSubmit={updateNetwork}
        />
      </div>
    </div>
  );
};

export default OSNetwork;
