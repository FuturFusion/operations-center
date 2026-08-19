import { FC, useState } from "react";
import { MdCancel } from "react-icons/md";
import { cancelClusterOperation } from "api/cluster";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { Cluster } from "types/cluster";
import { useQueryClient } from "@tanstack/react-query";

interface Props {
  cluster: Cluster;
  recommended?: boolean;
}

const ClusterCancelOperationBtn: FC<Props> = ({ cluster, recommended }) => {
  const [showModal, setShowModal] = useState(false);
  const [opInProgress, setOpInProgress] = useState(false);
  const { notify } = useNotification();
  const queryClient = useQueryClient();
  const actionStyle = {
    cursor: "pointer",
    color: recommended ? "red" : "grey",
  };

  const onCancelOperation = () => {
    setOpInProgress(true);
    cancelClusterOperation(cluster.name)
      .then((response) => {
        setOpInProgress(false);
        setShowModal(false);
        if (response.error_code == 0) {
          notify.success(`Cluster cancel operation triggered`);
          queryClient.invalidateQueries({ queryKey: ["clusters"] });
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        setOpInProgress(false);
        setShowModal(false);
        notify.error(`Error during cluster operation cancel: ${e}`);
      });
  };

  return (
    <>
      <MdCancel
        size={25}
        title="Cancel cluster operation"
        style={actionStyle}
        onClick={() => {
          setShowModal(true);
        }}
      />
      <ModalWindow
        show={showModal}
        scrollable
        handleClose={() => setShowModal(false)}
        title="Cancel cluster operation"
        footer={
          <>
            <LoadingButton
              isLoading={opInProgress}
              variant="danger"
              onClick={onCancelOperation}
            >
              Cancel
            </LoadingButton>
          </>
        }
      >
        <div>
          <div className="mb-3">
            Are you sure that you want to cancel the ongoing operation of
            cluster "{cluster.name}"?
          </div>
        </div>
      </ModalWindow>
    </>
  );
};

export default ClusterCancelOperationBtn;
