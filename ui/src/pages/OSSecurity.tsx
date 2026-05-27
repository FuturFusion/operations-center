import type { FC } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { fetchOSSecurity, updateOSSecurity } from "api/os";
import YAMLEditor from "components/YAMLEditor";
import { useNotification } from "context/notificationContext";
import type { IncusOSConfig } from "types/os";
import YAML from "yaml";

const OSSecurity: FC = () => {
  const queryClient = useQueryClient();
  const { notify } = useNotification();

  const {
    data: securityData = "",
    isLoading,
    error,
  } = useQuery({
    queryKey: ["os-security"],
    queryFn: async () => fetchOSSecurity(),
  });

  const updateSecurity = (value: string): Promise<boolean> => {
    let securityConfig = {} as IncusOSConfig;

    try {
      securityConfig = YAML.parse(value);
    } catch (error) {
      notify.error(`Error during YAML value parsing: ${error}`);
      return Promise.resolve(false);
    }

    return updateOSSecurity(JSON.stringify({ config: securityConfig.config }))
      .then(() => {
        notify.success(`Security updated`);
        queryClient.invalidateQueries({ queryKey: ["os-security"] });
        return true;
      })
      .catch((e) => {
        notify.error(`Security update failed: ${e}`);
        return false;
      });
  };

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading security data</div>;
  }

  return (
    <div className="d-flex flex-column" style={{ height: "70vh" }}>
      <div className="flex-grow-1">
        <YAMLEditor
          yamlData={YAML.stringify(securityData, null, 2)}
          onSubmit={updateSecurity}
        />
      </div>
    </div>
  );
};

export default OSSecurity;
