import { FC, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { MdCancel } from "react-icons/md";
import { cancelServerDeployment } from "api/server";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { Server } from "types/server";

interface Props {
  server: Server;
}

const ServerDeployCancelBtn: FC<Props> = ({ server }) => {
  const [showModal, setShowModal] = useState(false);
  const [opInProgress, setOpInProgress] = useState(false);
  const { notify } = useNotification();
  const queryClient = useQueryClient();

  const actionStyle = {
    cursor: "pointer",
    color: "grey",
  };

  const onCancelDeployment = () => {
    setOpInProgress(true);
    cancelServerDeployment(server.name)
      .then(() => {
        setOpInProgress(false);
        setShowModal(false);
        notify.success(`Cancellation of the deployment triggered`);
        queryClient.invalidateQueries({ queryKey: ["servers"] });
      })
      .catch((e) => {
        setOpInProgress(false);
        notify.error(`Error during deployment cancellation: ${e}`);
      });
  };

  return (
    <>
      <MdCancel
        size={25}
        title="Cancel deployment"
        style={actionStyle}
        onClick={() => {
          setShowModal(true);
        }}
      />
      <ModalWindow
        show={showModal}
        handleClose={() => setShowModal(false)}
        title="Cancel deployment"
        footer={
          <LoadingButton
            isLoading={opInProgress}
            variant="danger"
            onClick={onCancelDeployment}
          >
            Cancel deployment
          </LoadingButton>
        }
      >
        <p>
          Are you sure you want to cancel the deployment of the server "
          {server.name}"?
          <br />
          The installation media is ejected and the server is powered off.
        </p>
      </ModalWindow>
    </>
  );
};

export default ServerDeployCancelBtn;
