import { FC, useState } from "react";
import Button from "react-bootstrap/Button";
import { useQueryClient } from "@tanstack/react-query";
import { importUpdates } from "api/update";
import FileUploader from "components/FileUploader";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";

const ImportUpdatesBtn: FC = () => {
  const [showModal, setShowModal] = useState(false);
  const queryClient = useQueryClient();
  const { notify } = useNotification();

  const onImport = async (file: File | null) => {
    return await importUpdates(file)
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Updates imported`);
          queryClient.invalidateQueries({ queryKey: ["updates"] });
          setShowModal(false);
          return true;
        }
        notify.error(response.error);
        return false;
      })
      .catch((e) => {
        notify.error(`Error during importing updates: ${e}`);
        return false;
      });
  };

  return (
    <>
      <Button
        variant="success"
        className="float-end mx-2"
        onClick={() => setShowModal(true)}
      >
        Import
      </Button>
      <ModalWindow
        show={showModal}
        scrollable
        handleClose={() => setShowModal(false)}
        title="Import updates"
        footer={<></>}
      >
        <FileUploader onUpload={onImport} />
      </ModalWindow>
    </>
  );
};

export default ImportUpdatesBtn;
