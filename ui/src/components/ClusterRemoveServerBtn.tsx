import { FC, useState } from "react";
import { Form } from "react-bootstrap";
import { MdRemoveCircleOutline } from "react-icons/md";
import { useQueryClient } from "@tanstack/react-query";
import { clusterRemoveServer } from "api/cluster";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import ServerSelect from "components/ServerSelect";
import { useNotification } from "context/notificationContext";
import { useServers } from "context/useServers";
import { Cluster } from "types/cluster";
import { errorMessage } from "util/response";

interface Props {
  cluster: Cluster;
  recommended?: boolean;
}

const ClusterRemoveServerBtn: FC<Props> = ({ cluster, recommended }) => {
  const { data: servers } = useServers(`cluster=='${cluster.name}'`);
  const [showModal, setShowModal] = useState(false);
  const [opInProgress, setOpInProgress] = useState(false);
  const [serverNames, setServerNames] = useState<string[]>([]);
  const [force, setForce] = useState(false);
  const { notify } = useNotification();
  const queryClient = useQueryClient();
  const actionStyle = {
    cursor: "pointer",
    color: recommended ? "red" : "grey",
  };

  const onServerRemove = () => {
    setOpInProgress(true);
    clusterRemoveServer(
      cluster.name,
      JSON.stringify({ server_names: serverNames, force: force }, null, 2),
    )
      .then((response) => {
        setOpInProgress(false);
        setShowModal(false);
        if (response.error_code == 0) {
          notify.success(`Server removed from cluster`);
          queryClient.invalidateQueries({ queryKey: ["servers"] });
          return;
        }
        notify.error(errorMessage(response));
      })
      .catch((e) => {
        setOpInProgress(false);
        setShowModal(false);
        notify.error(`Error during server removal: ${e}`);
      });
  };

  return (
    <>
      <MdRemoveCircleOutline
        size={25}
        title="Remove server"
        style={actionStyle}
        onClick={() => {
          setShowModal(true);
        }}
      />
      <ModalWindow
        show={showModal}
        scrollable
        handleClose={() => setShowModal(false)}
        title="Remove server"
        footer={
          <>
            <LoadingButton
              isLoading={opInProgress}
              variant="danger"
              onClick={onServerRemove}
            >
              Remove
            </LoadingButton>
          </>
        }
      >
        <div>
          <div className="my-3">
            <Form.Group className="mb-4" controlId="serverNames">
              <Form.Label>Servers</Form.Label>
              <ServerSelect
                selected={serverNames}
                servers={servers ?? []}
                disabled={opInProgress}
                onChange={(values: string[]) => setServerNames(values)}
              />
            </Form.Group>
            <Form.Group className="mb-3" controlId="force">
              <Form.Check
                type="checkbox"
                label="Remove the servers even if they are unreachable or not evacuated"
                name="force"
                checked={force}
                disabled={opInProgress}
                onChange={(e) => setForce(e.target.checked)}
              />
              <Form.Text>
                The local instances and custom storage volumes of the removed
                servers will be lost and the servers will not be factory reset.
                The server records will be kept, no longer being part of the
                cluster.
              </Form.Text>
            </Form.Group>
          </div>
        </div>
      </ModalWindow>
    </>
  );
};

export default ClusterRemoveServerBtn;
