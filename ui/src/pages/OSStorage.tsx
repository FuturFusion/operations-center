import type { FC } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { fetchOSStorage, updateOSStorage } from "api/os";
import YAMLEditor from "components/YAMLEditor";
import { useNotification } from "context/notificationContext";
import type { IncusOSConfig } from "types/os";
import YAML from "yaml";

const OSStorage: FC = () => {
  const queryClient = useQueryClient();
  const { notify } = useNotification();

  const {
    data: storageData = "",
    isLoading,
    error,
  } = useQuery({
    queryKey: ["os-storage"],
    queryFn: async () => fetchOSStorage(),
  });

  const updateStorage = (value: string): Promise<boolean> => {
    let storageConfig = {} as IncusOSConfig;

    try {
      storageConfig = YAML.parse(value);
    } catch (error) {
      notify.error(`Error during YAML value parsing: ${error}`);
      return Promise.resolve(false);
    }

    return updateOSStorage(JSON.stringify({ config: storageConfig.config }))
      .then(() => {
        notify.success(`Network updated`);
        queryClient.invalidateQueries({ queryKey: ["os-storage"] });
        return true;
      })
      .catch((e) => {
        notify.error(`Storage update failed: ${e}`);
        return false;
      });
  };

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading storage data</div>;
  }

  return (
    <div className="d-flex flex-column" style={{ height: "70vh" }}>
      <div className="flex-grow-1">
        <YAMLEditor
          yamlData={YAML.stringify(storageData, null, 2)}
          onSubmit={updateStorage}
        />
      </div>
    </div>
  );
};

export default OSStorage;
