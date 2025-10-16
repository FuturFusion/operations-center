import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import {
  fetchClusterTemplate,
  renameClusterTemplate,
  updateClusterTemplate,
} from "api/cluster_template";
import ClusterTemplateForm from "components/ClusterTemplateForm";
import { useNotification } from "context/notificationContext";
import { ClusterTemplateFormValues } from "types/cluster_template";

const ClusterTemplateConfiguration = () => {
  const { name } = useParams() as { name: string };
  const { notify } = useNotification();
  const navigate = useNavigate();

  const onSubmit = (values: ClusterTemplateFormValues) => {
    updateClusterTemplate(name, JSON.stringify(values, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Cluster template ${name} updated`);
          navigate(`/ui/provisioning/cluster-templates/${name}/configuration`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during cluster template update: ${e}`);
      });
  };

  const onRename = (newName: string) => {
    if (name !== newName) {
      renameClusterTemplate(name, JSON.stringify({ name: newName }, null, 2))
        .then((response) => {
          if (response.error_code == 0) {
            notify.success(`Cluster template ${newName} renamed`);
            navigate(
              `/ui/provisioning/cluster-templates/${newName}/configuration`,
            );
            return;
          }
          notify.error(response.error);
        })
        .catch((e) => {
          notify.error(`Error during cluster template rename: ${e}`);
        });
    }
  };

  const {
    data: template = undefined,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["template", name],
    queryFn: () => fetchClusterTemplate(name),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading cluster template</div>;
  }

  return (
    <ClusterTemplateForm
      clusterTemplate={template}
      onSubmit={onSubmit}
      onRename={onRename}
    />
  );
};

export default ClusterTemplateConfiguration;
