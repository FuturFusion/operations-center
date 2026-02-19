import { FC, useState } from "react";
import { MdRestore } from "react-icons/md";
import { restoreServer } from "api/server";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { Server } from "types/server";

interface Props {
  server: Server;
  recommended?: boolean;
}

const ServerRestoreBtn: FC<Props> = ({ server, recommended }) => {
  const [showModal, setShowModal] = useState(false);
  const [opInProgress, setOpInProgress] = useState(false);
  const { notify } = useNotification();
  const actionStyle = {
    cursor: "pointer",
    color: recommended ? "red" : "grey",
  };

  const onRestoreServer = () => {
    setOpInProgress(true);
    restoreServer(server.name)
      .then((response) => {
        setOpInProgress(false);
        setShowModal(false);
        if (response.error_code == 0) {
          notify.success(`Server restore triggered`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        setOpInProgress(false);
        setShowModal(false);
        notify.error(`Error during server restore: ${e}`);
      });
  };

  return (
    <>
      <MdRestore
        size={25}
        title="Restore server"
        style={actionStyle}
        onClick={() => {
          setShowModal(true);
        }}
      />
      <ModalWindow
        show={showModal}
        scrollable
        handleClose={() => setShowModal(false)}
        title="Restore server"
        footer={
          <>
            <LoadingButton
              isLoading={opInProgress}
              variant="danger"
              onClick={onRestoreServer}
            >
              Restore
            </LoadingButton>
          </>
        }
      >
        <p>Are you sure you want to restore the server "{server.name}"?</p>
      </ModalWindow>
    </>
  );
};

export default ServerRestoreBtn;
