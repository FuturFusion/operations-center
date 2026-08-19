import { FC, useState } from "react";
import { MdRestartAlt } from "react-icons/md";
import { rebootClusterRolling } from "api/cluster";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { Cluster } from "types/cluster";
import { useQueryClient } from "@tanstack/react-query";

interface Props {
  cluster: Cluster;
  recommended?: boolean;
}

const ClusterRebootBtn: FC<Props> = ({ cluster, recommended }) => {
  const [showModal, setShowModal] = useState(false);
  const [opInProgress, setOpInProgress] = useState(false);
  const { notify } = useNotification();
  const queryClient = useQueryClient();
  const actionStyle = {
    cursor: "pointer",
    color: recommended ? "red" : "grey",
  };

  const onRebootCluster = () => {
    setOpInProgress(true);
    rebootClusterRolling(cluster.name)
      .then((response) => {
        setOpInProgress(false);
        setShowModal(false);
        if (response.error_code == 0) {
          notify.success(`Cluster rolling reboot triggered`);
          queryClient.invalidateQueries({ queryKey: ["clusters"] });
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        setOpInProgress(false);
        setShowModal(false);
        notify.error(`Error during cluster rolling reboot: ${e}`);
      });
  };

  return (
    <>
      <MdRestartAlt
        size={25}
        title="Reboot cluster"
        style={actionStyle}
        onClick={() => {
          setShowModal(true);
        }}
      />
      <ModalWindow
        show={showModal}
        scrollable
        handleClose={() => setShowModal(false)}
        title="Reboot cluster"
        footer={
          <>
            <LoadingButton
              isLoading={opInProgress}
              variant="danger"
              onClick={onRebootCluster}
            >
              Reboot
            </LoadingButton>
          </>
        }
      >
        <div>
          <div className="mb-3">
            Are you sure you want to perform a rolling reboot of the cluster "
            {cluster.name}"?
          </div>
          <div className="mb-3">
            Every server of the cluster is evacuated, rebooted and restored
            again, one server at a time. If an OS update has already been staged
            on a server, the reboot applies it.
          </div>
        </div>
      </ModalWindow>
    </>
  );
};

export default ClusterRebootBtn;
