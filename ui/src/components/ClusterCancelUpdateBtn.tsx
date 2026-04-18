import { FC, useState } from "react";
import { MdCancel } from "react-icons/md";
import { cancelUpdateClusterRolling } from "api/cluster";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { Cluster } from "types/cluster";
import { useQueryClient } from "@tanstack/react-query";

interface Props {
  cluster: Cluster;
  recommended?: boolean;
}

const ClusterCancelUpdateBtn: FC<Props> = ({ cluster, recommended }) => {
  const [showModal, setShowModal] = useState(false);
  const [opInProgress, setOpInProgress] = useState(false);
  const { notify } = useNotification();
  const queryClient = useQueryClient();
  const actionStyle = {
    cursor: "pointer",
    color: recommended ? "red" : "grey",
  };

  const onCancelUpdate = () => {
    setOpInProgress(true);
    cancelUpdateClusterRolling(cluster.name)
      .then((response) => {
        setOpInProgress(false);
        setShowModal(false);
        if (response.error_code == 0) {
          notify.success(`Cluster cancel update triggered`);
          queryClient.invalidateQueries({ queryKey: ["clusters"] });
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        setOpInProgress(false);
        setShowModal(false);
        notify.error(`Error during cluster update cancel: ${e}`);
      });
  };

  return (
    <>
      <MdCancel
        size={25}
        title="Cancel cluster update"
        style={actionStyle}
        onClick={() => {
          setShowModal(true);
        }}
      />
      <ModalWindow
        show={showModal}
        scrollable
        handleClose={() => setShowModal(false)}
        title="Cancel cluster update"
        footer={
          <>
            <LoadingButton
              isLoading={opInProgress}
              variant="danger"
              onClick={onCancelUpdate}
            >
              Cancel
            </LoadingButton>
          </>
        }
      >
        <div>
          <div className="mb-3">
            Are you sure that you want to cancel the ongoing update of cluster "
            {cluster.name}"?
          </div>
        </div>
      </ModalWindow>
    </>
  );
};

export default ClusterCancelUpdateBtn;
