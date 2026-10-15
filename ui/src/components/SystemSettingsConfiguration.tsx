import { useQuery } from "@tanstack/react-query";
import { fetchSystemSettings, updateSystemSettings } from "api/settings";
import SystemSettingsForm from "components/SystemSettingsForm";
import { useNotification } from "context/notificationContext";
import { SystemSettings } from "types/settings";
import { errorMessage } from "util/response";
import QueryError from "components/QueryError";

const SystemSettingsConfiguration = () => {
  const { notify } = useNotification();

  const onSubmit = (settings: SystemSettings) => {
    return updateSystemSettings(JSON.stringify(settings, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`System settings updated`);
          return;
        }
        notify.error(errorMessage(response));
      })
      .catch((e) => {
        notify.error(`Error during system settings update: ${e}`);
      });
  };

  const {
    data: settings = undefined,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["system_settings"],
    queryFn: () => fetchSystemSettings(),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <QueryError error={error} label="the system settings" />;
  }

  return <SystemSettingsForm settings={settings} onSubmit={onSubmit} />;
};

export default SystemSettingsConfiguration;
