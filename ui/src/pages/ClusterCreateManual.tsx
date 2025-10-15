import { useNavigate } from "react-router";
import { createCluster } from "api/cluster";
import ClusterCreateManualForm from "components/ClusterCreateManualForm";
import { useNotification } from "context/notificationContext";
import { ClusterFormValues } from "types/cluster";

const ClusterCreateManual = () => {
  const { notify } = useNotification();
  const navigate = useNavigate();

  const onSubmit = (values: ClusterFormValues) => {
    console.log(values);
    createCluster(JSON.stringify(values, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Cluster ${values.name} created`);
          navigate(`/ui/provisioning/clusters`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during cluster creation: ${e}`);
      });
  };

  return <ClusterCreateManualForm onSubmit={onSubmit} />;
};

export default ClusterCreateManual;
