import { useQuery } from "@tanstack/react-query";
import { fetchSystemUpdates, updateSystemUpdates } from "api/settings";
import SystemUpdatesForm from "components/SystemUpdatesForm";
import { useNotification } from "context/notificationContext";
import { SystemUpdates } from "types/settings";
import { errorMessage } from "util/response";

const SystemUpdatesConfiguration = () => {
  const { notify } = useNotification();

  const onSubmit = (updates: SystemUpdates) => {
    updateSystemUpdates(JSON.stringify(updates, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`System updates updated`);
          return;
        }
        notify.error(errorMessage(response));
      })
      .catch((e) => {
        notify.error(`Error during system updates update: ${e}`);
      });
  };

  const {
    data: updates = undefined,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["system_updates"],
    queryFn: () => fetchSystemUpdates(),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading system updates</div>;
  }

  return <SystemUpdatesForm updates={updates} onSubmit={onSubmit} />;
};

export default SystemUpdatesConfiguration;
