import { FC } from "react";
import { Button } from "react-bootstrap";
import { useFormik } from "formik";
import { updateClusterCert } from "api/cluster";
import ClusterCertForm from "components/ClusterCertForm";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { Cluster, ClusterCertFormValues } from "types/cluster";

interface Props {
  cluster: Cluster;
  show: boolean;
  handleClose: () => void;
}

const ClusterUpdateCertModal: FC<Props> = ({ cluster, show, handleClose }) => {
  const { notify } = useNotification();
  const formikInitialValues: ClusterCertFormValues = {
    cluster_certificate: "",
    cluster_certificate_key: "",
  };

  const formik = useFormik({
    initialValues: formikInitialValues,
    onSubmit: (values: ClusterCertFormValues, { resetForm }) => {
      handleClose();
      updateClusterCert(cluster.name, JSON.stringify(values, null, 2))
        .then((response) => {
          if (response.error_code == 0) {
            notify.success(`Cluster ${name} certificate updated`);
            return;
          }
          notify.error(response.error);
        })
        .catch((e) => {
          notify.error(`Error during token update: ${e}`);
        });
      resetForm();
    },
  });

  return (
    <ModalWindow
      show={show}
      handleClose={handleClose}
      title="Update the cluster certificate"
      footer={
        <>
          <Button variant="success" onClick={formik.submitForm}>
            Update
          </Button>
        </>
      }
    >
      <ClusterCertForm formik={formik} />
    </ModalWindow>
  );
};

export default ClusterUpdateCertModal;
