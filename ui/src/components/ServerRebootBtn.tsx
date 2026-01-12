import { FC, useState } from "react";
import { MdOutlineReplay } from "react-icons/md";
import { rebootServer } from "api/server";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { Server } from "types/server";

interface Props {
  server: Server;
}

const ServerRebootBtn: FC<Props> = ({ server }) => {
  const [showModal, setShowModal] = useState(false);
  const [rebootInProgress, setRebootInProgress] = useState(false);
  const { notify } = useNotification();
  const actionStyle = {
    cursor: "pointer",
    color: "grey",
  };

  const onRebootServer = () => {
    setRebootInProgress(true);
    rebootServer(server.name)
      .then((response) => {
        setRebootInProgress(false);
        setShowModal(false);
        if (response.error_code == 0) {
          notify.success(`Server resync triggered`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        setRebootInProgress(false);
        setShowModal(false);
        notify.error(`Error during server sync: ${e}`);
      });
  };

  return (
    <>
      <MdOutlineReplay
        size={25}
        title="Reboot server"
        style={actionStyle}
        onClick={() => {
          setShowModal(true);
        }}
      />
      <ModalWindow
        show={showModal}
        scrollable
        handleClose={() => setShowModal(false)}
        title="Reboot server"
        footer={
          <>
            <LoadingButton
              isLoading={rebootInProgress}
              variant="danger"
              onClick={onRebootServer}
            >
              Reboot
            </LoadingButton>
          </>
        }
      >
        <p>Are you sure you want to reboot the server "{server.name}"?</p>
      </ModalWindow>
    </>
  );
};

export default ServerRebootBtn;
