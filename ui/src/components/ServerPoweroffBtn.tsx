import { FC, useState } from "react";
import { MdPowerSettingsNew } from "react-icons/md";
import { poweroffServer } from "api/server";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { Server } from "types/server";

interface Props {
  server: Server;
}

const ServerPoweroffBtn: FC<Props> = ({ server }) => {
  const [showModal, setShowModal] = useState(false);
  const [poweroffInProgress, setPoweroffInProgress] = useState(false);
  const { notify } = useNotification();
  const actionStyle = {
    cursor: "pointer",
    color: "grey",
  };

  const onPoweroffServer = () => {
    setPoweroffInProgress(true);
    poweroffServer(server.name)
      .then((response) => {
        setPoweroffInProgress(false);
        setShowModal(false);
        if (response.error_code == 0) {
          notify.success(`Server poweroff triggered`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        setPoweroffInProgress(false);
        setShowModal(false);
        notify.error(`Error during server poweroff: ${e}`);
      });
  };

  return (
    <>
      <MdPowerSettingsNew
        size={25}
        title="Power off server"
        style={actionStyle}
        onClick={() => {
          setShowModal(true);
        }}
      />
      <ModalWindow
        show={showModal}
        scrollable
        handleClose={() => setShowModal(false)}
        title="Power off server"
        footer={
          <>
            <LoadingButton
              isLoading={poweroffInProgress}
              variant="danger"
              onClick={onPoweroffServer}
            >
              Power off
            </LoadingButton>
          </>
        }
      >
        <p>Are you sure you want to power off the server "{server.name}"?</p>
      </ModalWindow>
    </>
  );
};

export default ServerPoweroffBtn;
