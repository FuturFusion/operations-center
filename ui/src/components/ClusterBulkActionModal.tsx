import { FC } from "react";
import { Button } from "react-bootstrap";
import { useFormik } from "formik";
import { bulkClusterAction } from "api/cluster";
import ClusterBulkUpdateForm from "components/ClusterBulkUpdateForm";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { Cluster, ClusterBulkUpdateFormValues } from "types/cluster";
import YAML from "yaml";

interface Props {
  cluster: Cluster;
  show: boolean;
  handleClose: () => void;
}

const ClusterBulkActionModal: FC<Props> = ({ cluster, show, handleClose }) => {
  const { notify } = useNotification();
  const formikInitialValues: ClusterBulkUpdateFormValues = {
    action: "",
    arguments: "",
  };

  const formik = useFormik({
    initialValues: formikInitialValues,
    onSubmit: (values: ClusterBulkUpdateFormValues, { resetForm }) => {
      let argumentsValue = {};

      try {
        argumentsValue = YAML.parse(values.arguments);
      } catch (error) {
        notify.error(`Error during YAML value parsing: ${error}`);
        return;
      }

      bulkClusterAction(
        cluster.name,
        JSON.stringify(
          { action: values.action, arguments: argumentsValue },
          null,
          2,
        ),
      )
        .then((response) => {
          if (response.error_code == 0) {
            notify.success(`Bulk action completed on cluster ${name}`);
            resetForm();
            handleClose();
            return;
          }
          notify.error(response.error);
        })
        .catch((e) => {
          notify.error(`Error during token update: ${e}`);
        });
    },
  });

  return (
    <ModalWindow
      show={show}
      handleClose={handleClose}
      title="Bulk action"
      footer={
        <>
          <Button variant="success" onClick={formik.submitForm}>
            Run
          </Button>
        </>
      }
    >
      <ClusterBulkUpdateForm formik={formik} />
    </ModalWindow>
  );
};

export default ClusterBulkActionModal;
