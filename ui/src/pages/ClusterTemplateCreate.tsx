import { useNavigate } from "react-router";
import { useNotification } from "context/notificationContext";
import { createClusterTemplate } from "api/cluster_template";
import ClusterTemplateForm from "components/ClusterTemplateForm";
import { ClusterTemplateFormValues } from "types/cluster_template";

const ClusterTemplateCreate = () => {
  const { notify } = useNotification();
  const navigate = useNavigate();

  const onSubmit = (values: ClusterTemplateFormValues) => {
    createClusterTemplate(JSON.stringify(values, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Cluster template ${values.name} created`);
          navigate("/ui/provisioning/clusters-view/templates");
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during cluster template creation: ${e}`);
      });
  };

  return (
    <div className="d-flex flex-column">
      <div className="scroll-container flex-grow-1 p-3">
        <ClusterTemplateForm onSubmit={onSubmit} />
      </div>
    </div>
  );
};

export default ClusterTemplateCreate;
