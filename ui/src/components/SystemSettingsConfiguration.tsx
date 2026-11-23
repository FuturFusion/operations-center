import { useQuery } from "@tanstack/react-query";
import { fetchSystemSettings, updateSystemSettings } from "api/settings";
import SystemSettingsForm from "components/SystemSettingsForm";
import { useNotification } from "context/notificationContext";
import { SystemSettings } from "types/settings";
import { errorMessage } from "util/response";
import QueryError from "components/QueryError";

const SystemSettingsConfiguration = () => {
  const { notify } = useNotification();

  const {
    data: result = undefined,
    error,
    isLoading,
    refetch,
  } = useQuery({
    queryKey: ["system_settings"],
    queryFn: () => fetchSystemSettings(),
  });

  const onSubmit = (settings: SystemSettings) => {
    return (
      updateSystemSettings(
        JSON.stringify(settings, null, 2),
        result?.etag ?? "",
      )
        .then((response) => {
          if (response.error_code == 0) {
            notify.success(`System settings updated`);
            return;
          }
          notify.error(errorMessage(response));
        })
        .catch((e) => {
          notify.error(`Error during system settings update: ${e}`);
        })
        // The ETag changes with every update, after a refused one the current config is shown.
        .finally(() => refetch())
    );
  };

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <QueryError error={error} label="the system settings" />;
  }

  return <SystemSettingsForm settings={result?.data} onSubmit={onSubmit} />;
};

export default SystemSettingsConfiguration;
