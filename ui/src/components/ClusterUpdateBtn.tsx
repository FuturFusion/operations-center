import { FC, useState } from "react";
import { MdSystemUpdateAlt } from "react-icons/md";
import { updateClusterRolling } from "api/cluster";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { Cluster } from "types/cluster";
import { useQueryClient } from "@tanstack/react-query";
import { Form } from "react-bootstrap";

interface Props {
  cluster: Cluster;
  recommended?: boolean;
}

const ClusterUpdateBtn: FC<Props> = ({ cluster, recommended }) => {
  const [showModal, setShowModal] = useState(false);
  const [opInProgress, setOpInProgress] = useState(false);
  const [reboot, setReboot] = useState(true);
  const { notify } = useNotification();
  const queryClient = useQueryClient();
  const actionStyle = {
    cursor: "pointer",
    color: recommended ? "red" : "grey",
  };

  const onUpdateCluster = () => {
    setOpInProgress(true);
    updateClusterRolling(
      cluster.name,
      JSON.stringify({ reboot: reboot }, null, 2),
    )
      .then((response) => {
        setOpInProgress(false);
        setShowModal(false);
        if (response.error_code == 0) {
          notify.success(`Cluster update triggered`);
          queryClient.invalidateQueries({ queryKey: ["clusters"] });
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        setOpInProgress(false);
        setShowModal(false);
        notify.error(`Error during cluster update: ${e}`);
      });
  };

  return (
    <>
      <MdSystemUpdateAlt
        size={25}
        title="Update cluster"
        style={actionStyle}
        onClick={() => {
          setShowModal(true);
        }}
      />
      <ModalWindow
        show={showModal}
        scrollable
        handleClose={() => setShowModal(false)}
        title="Update cluster"
        footer={
          <>
            <LoadingButton
              isLoading={opInProgress}
              variant="danger"
              onClick={onUpdateCluster}
            >
              Update
            </LoadingButton>
          </>
        }
      >
        <div>
          <div className="mb-3">
            Are you sure you want to update the cluster "{cluster.name}"?
          </div>
          <div className="my-3">
            <Form.Group
              controlId="reboot"
              className="mb-3 d-flex align-items-center gap-2"
            >
              <Form.Check
                type="checkbox"
                name="acme.agree_tos"
                checked={reboot}
                onChange={(e) => setReboot(e.target.checked)}
              />
              <Form.Label className="me-2 mb-0">
                Perform a rolling reboot following the installation of the
                update
              </Form.Label>
            </Form.Group>
          </div>
        </div>
      </ModalWindow>
    </>
  );
};

export default ClusterUpdateBtn;
