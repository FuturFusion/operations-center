import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import { fetchServer, renameServer } from "api/server";
import ServerForm from "components/ServerForm";
import { useNotification } from "context/notificationContext";
import { ServerFormValues } from "types/server";

const ServerConfiguration = () => {
  const { name } = useParams() as { name: string };
  const { notify } = useNotification();
  const navigate = useNavigate();

  const onSubmit = (values: ServerFormValues) => {
    renameServer(name, JSON.stringify(values, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Server ${name} updated`);
          navigate(`/ui/provisioning/servers/${values.name}/configuration`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during server update: ${e}`);
      });
  };

  const {
    data: server = undefined,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["servers", name],
    queryFn: () => fetchServer(name),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading servers</div>;
  }

  return <ServerForm server={server} onSubmit={onSubmit} />;
};

export default ServerConfiguration;
