import { useQuery } from "@tanstack/react-query";
import { fetchSystemSecurity, updateSystemSecurity } from "api/settings";
import SystemSecurityForm from "components/SystemSecurityForm";
import { useNotification } from "context/notificationContext";
import { SystemSecurity } from "types/settings";
import { errorMessage } from "util/response";
import QueryError from "components/QueryError";

const SystemSecurityConfiguration = () => {
  const { notify } = useNotification();

  const {
    data: result = undefined,
    error,
    isLoading,
    refetch,
  } = useQuery({
    queryKey: ["system_security"],
    queryFn: () => fetchSystemSecurity(),
  });

  const onSubmit = (security: SystemSecurity) => {
    updateSystemSecurity(JSON.stringify(security, null, 2), result?.etag ?? "")
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`System security updated`);
          return;
        }
        notify.error(errorMessage(response));
      })
      .catch((e) => {
        notify.error(`Error during system security update: ${e}`);
      })
      // The ETag changes with every update, after a refused one the current config is shown.
      .finally(() => refetch());
  };

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <QueryError error={error} label="the security settings" />;
  }

  return <SystemSecurityForm security={result?.data} onSubmit={onSubmit} />;
};

export default SystemSecurityConfiguration;
