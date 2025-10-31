import { useQuery } from "@tanstack/react-query";
import { fetchSystemSecurity, updateSystemSecurity } from "api/settings";
import SystemSecurityForm from "components/SystemSecurityForm";
import { useNotification } from "context/notificationContext";
import { SystemSecurity } from "types/settings";

const SystemSecurityConfiguration = () => {
  const { notify } = useNotification();

  const onSubmit = (security: SystemSecurity) => {
    updateSystemSecurity(JSON.stringify(security, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`System security updated`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during system security update: ${e}`);
      });
  };

  const {
    data: security = undefined,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["system_security"],
    queryFn: () => fetchSystemSecurity(),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading system security</div>;
  }

  return <SystemSecurityForm security={security} onSubmit={onSubmit} />;
};

export default SystemSecurityConfiguration;
