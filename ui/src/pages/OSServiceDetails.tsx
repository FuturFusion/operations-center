import type { FC } from "react";
import { useParams } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { fetchOSService, updateOSService } from "api/os";
import Breadcrumbs from "components/Breadcrumbs";
import YAMLEditor from "components/YAMLEditor";
import { useNotification } from "context/notificationContext";
import type { IncusOSConfig } from "types/os";
import YAML from "yaml";

const OSServiceDetails: FC = () => {
  const queryClient = useQueryClient();
  const { notify } = useNotification();

  const { name } = useParams<{
    name: string;
  }>();

  const {
    data: serviceData,
    isLoading,
    error,
  } = useQuery({
    queryKey: ["os-service-details", name],
    queryFn: async () => fetchOSService(name ?? ""),
  });

  const updateService = (value: string): Promise<boolean> => {
    let serviceConfig = {} as IncusOSConfig;

    try {
      serviceConfig = YAML.parse(value);
    } catch (error) {
      notify.error(`Error during YAML value parsing: ${error}`);
      return Promise.resolve(false);
    }

    return updateOSService(
      name ?? "",
      JSON.stringify({ config: serviceConfig.config }),
    )
      .then(() => {
        notify.success(`Service updated`);
        queryClient.invalidateQueries({ queryKey: ["os-service-details"] });
        return true;
      })
      .catch((e) => {
        notify.error(`Service update failed: ${e}`);
        return false;
      });
  };

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading service data</div>;
  }

  return (
    <div className="d-flex flex-column" style={{ height: "80vh" }}>
      <Breadcrumbs />
      <div className="flex-grow-1 p-4">
        <YAMLEditor
          yamlData={YAML.stringify(serviceData, null, 2)}
          onSubmit={updateService}
        />
      </div>
    </div>
  );
};

export default OSServiceDetails;
