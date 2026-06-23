import type { FC } from "react";
import { useState } from "react";
import { MdOutlineReplay } from "react-icons/md";
import { rebootOS } from "api/os";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";

interface Props {
  pending?: boolean;
}

const RebootOSBtn: FC<Props> = ({ pending = false }) => {
  const { notify } = useNotification();
  const [showModal, setShowModal] = useState(false);
  const [rebootInProgress, setRebootInProgress] = useState(false);

  const handleReboot = () => {
    setRebootInProgress(true);
    rebootOS()
      .then(() => {
        notify.success(`OS rebooted`);
        setRebootInProgress(false);
        setShowModal(false);
      })
      .catch((e) => {
        notify.error(`OS reboot failed: ${e}`);
        setRebootInProgress(false);
        setShowModal(false);
      });
  };

  return (
    <>
      <MdOutlineReplay
        size={25}
        title={pending ? "Reboot required to apply update" : "Reboot server"}
        style={{ color: pending ? "#c00" : "grey", cursor: "pointer" }}
        onClick={() => setShowModal(true)}
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
              onClick={handleReboot}
            >
              Reboot
            </LoadingButton>
          </>
        }
      >
        <p>Are you sure you want to reboot the server ?</p>
      </ModalWindow>
    </>
  );
};

export default RebootOSBtn;
