import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import { fetchCluster, renameCluster, updateCluster } from "api/cluster";
import ClusterForm from "components/ClusterForm";
import { useNotification } from "context/notificationContext";
import { ClusterFormValues } from "types/cluster";

const ClusterConfiguration = () => {
  const { name } = useParams() as { name: string };
  const { notify } = useNotification();
  const navigate = useNavigate();

  const onSubmit = (values: ClusterFormValues) => {
    updateCluster(
      values.name,
      JSON.stringify(
        { connection_url: values.connection_url, channel: values.channel },
        null,
        2,
      ),
    )
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Cluster ${values.name} updated`);
          navigate(`/ui/provisioning/clusters/${values.name}/configuration`);
          return;
        }
        notify.error(`Error during cluster update: ${response.error}`);
      })
      .catch((e) => {
        notify.error(`Error during cluster update: ${e}`);
      });
  };

  const onRename = (newName: string) => {
    if (name !== newName) {
      renameCluster(name, JSON.stringify({ name: newName }, null, 2))
        .then((response) => {
          if (response.error_code == 0) {
            notify.success(`Cluster ${newName} renamed`);
            navigate(`/ui/provisioning/clusters/${newName}/configuration`);
            return;
          }
          notify.error(response.error);
        })
        .catch((e) => {
          notify.error(`Error during server rename: ${e}`);
        });
    }
  };

  const {
    data: cluster = undefined,
    error: error,
    isLoading: isLoading,
  } = useQuery({
    queryKey: ["clusters", name],
    queryFn: () => fetchCluster(name),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading cluster</div>;
  }

  return (
    <ClusterForm cluster={cluster} onRename={onRename} onSubmit={onSubmit} />
  );
};

export default ClusterConfiguration;
