import { FC, KeyboardEvent, useMemo, useState } from "react";
import { MdAddCircleOutline } from "react-icons/md";
import { clusterAddServers } from "api/cluster";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { useServers } from "context/useServers";
import { Cluster } from "types/cluster";
import { useQueryClient } from "@tanstack/react-query";
import { Form } from "react-bootstrap";
import { ServerType } from "util/server";
import { handleCtrlA } from "util/util";

interface Props {
  cluster: Cluster;
  recommended?: boolean;
}

const ClusterAddServerBtn: FC<Props> = ({ cluster, recommended }) => {
  const { data: servers } = useServers("cluster==nil");
  const [showModal, setShowModal] = useState(false);
  const [opInProgress, setOpInProgress] = useState(false);
  const [skipPostJoin, setSkipPostJoin] = useState(false);
  const [serverNames, setServerNames] = useState<string[]>([]);
  const { notify } = useNotification();
  const queryClient = useQueryClient();
  const actionStyle = {
    cursor: "pointer",
    color: recommended ? "red" : "grey",
  };

  const filteredServers = useMemo(
    () => servers?.filter((s) => s.server_type === ServerType.Incus),
    [servers],
  );

  const clearValues = () => {
    setSkipPostJoin(false);
    setServerNames([]);
  };

  const onServerAdd = () => {
    setOpInProgress(true);
    clusterAddServers(
      cluster.name,
      JSON.stringify(
        { skip_post_join_operations: skipPostJoin, server_names: serverNames },
        null,
        2,
      ),
    )
      .then((response) => {
        setOpInProgress(false);
        clearValues();
        setShowModal(false);
        if (response.error_code == 0) {
          notify.success(`Server added to cluster`);
          queryClient.invalidateQueries({ queryKey: ["clusters"] });
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        setOpInProgress(false);
        clearValues();
        setShowModal(false);
        notify.error(`Error during server adding: ${e}`);
      });
  };

  const handleServersKeyDown = (e: KeyboardEvent<HTMLSelectElement>) => {
    e.preventDefault();
    setServerNames(filteredServers?.map((s) => s.name) ?? []);
  };

  return (
    <>
      <MdAddCircleOutline
        size={25}
        title="Add server"
        style={actionStyle}
        onClick={() => {
          setShowModal(true);
        }}
      />
      <ModalWindow
        show={showModal}
        scrollable
        handleClose={() => setShowModal(false)}
        title="Add server"
        footer={
          <>
            <LoadingButton
              isLoading={opInProgress}
              variant="success"
              onClick={onServerAdd}
            >
              Add
            </LoadingButton>
          </>
        }
      >
        <div>
          <div className="my-3">
            <Form.Group className="mb-4" controlId="serverNames">
              <Form.Label>Servers</Form.Label>
              <Form.Select
                multiple
                value={serverNames}
                onChange={(e) => {
                  const selected = Array.from(
                    e.target.selectedOptions,
                    (option) => option.value,
                  );
                  setServerNames(selected);
                }}
                onKeyDown={handleCtrlA(handleServersKeyDown)}
                disabled={opInProgress}
              >
                {filteredServers?.map((server) => (
                  <option key={server.name} value={server.name}>
                    {server.name}
                  </option>
                ))}
              </Form.Select>
            </Form.Group>
            <Form.Group
              controlId="skipPostJoin"
              className="mb-3 d-flex align-items-center gap-2"
            >
              <Form.Check
                type="checkbox"
                name="skipPostJoin"
                checked={skipPostJoin}
                disabled={opInProgress}
                onChange={(e) => setSkipPostJoin(e.target.checked)}
              />
              <Form.Label className="me-2 mb-0">
                If set to true, the post join operations (namely the creation of
                the local storage volumes for backups, images and logs) are
                skipped.
              </Form.Label>
            </Form.Group>
          </div>
        </div>
      </ModalWindow>
    </>
  );
};

export default ClusterAddServerBtn;
