import type { FC } from "react";
import { useState } from "react";
import { MdPowerSettingsNew } from "react-icons/md";
import { poweroffOS } from "api/os";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";

const ShutdownOSBtn: FC = () => {
  const { notify } = useNotification();
  const [showModal, setShowModal] = useState(false);
  const [poweroffInProgress, setPoweroffInProgress] = useState(false);

  const handlePoweroff = () => {
    poweroffOS()
      .then(() => {
        notify.success(`OS shutdown`);
        setPoweroffInProgress(false);
        setShowModal(false);
      })
      .catch((e) => {
        notify.error(`OS poweroff failed: ${e}`);
        setPoweroffInProgress(false);
        setShowModal(false);
      });
  };

  return (
    <>
      <MdPowerSettingsNew
        size={25}
        title="Power off"
        style={{ color: "grey", cursor: "pointer" }}
        onClick={() => setShowModal(true)}
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
              onClick={handlePoweroff}
            >
              Power off
            </LoadingButton>
          </>
        }
      >
        <p>Are you sure you want to power off the server ?</p>
      </ModalWindow>
    </>
  );
};

export default ShutdownOSBtn;
