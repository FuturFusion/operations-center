import { FC, useState } from "react";
import Button from "react-bootstrap/Button";
import { BsArrowRepeat, BsTrash } from "react-icons/bs";
import { useQueryClient } from "@tanstack/react-query";
import { deleteImageSource, refreshImageSource } from "api/image_incus_source";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { ImageSource } from "types/image_incus_source";

interface Props {
  source: ImageSource;
}

const ImageSourceActions: FC<Props> = ({ source }) => {
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const queryClient = useQueryClient();
  const { notify } = useNotification();

  const actionStyle = {
    cursor: "pointer",
    color: "grey",
  };

  const handleRefresh = () => {
    refreshImageSource(source.name)
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Refresh of image source ${source.name} triggered`);
          queryClient.invalidateQueries({ queryKey: ["image-sources"] });
          queryClient.invalidateQueries({ queryKey: ["incus-images"] });
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during image source refresh: ${e}`);
      });
  };

  const handleDelete = () => {
    setShowDeleteModal(false);

    deleteImageSource(source.name)
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Image source ${source.name} deleted`);
          queryClient.invalidateQueries({ queryKey: ["image-sources"] });
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during image source deletion: ${e}`);
      });
  };

  return (
    <div className="d-flex gap-2">
      <BsArrowRepeat
        title="Refresh"
        style={actionStyle}
        onClick={handleRefresh}
      />
      <BsTrash
        title="Delete"
        style={actionStyle}
        onClick={() => setShowDeleteModal(true)}
      />
      <ModalWindow
        show={showDeleteModal}
        handleClose={() => setShowDeleteModal(false)}
        title="Delete image source?"
        footer={
          <>
            <Button variant="danger" onClick={handleDelete}>
              Delete
            </Button>
          </>
        }
      >
        <p>
          Are you sure you want to delete the image source "{source.name}"?
          <br />
          This action cannot be undone.
        </p>
      </ModalWindow>
    </div>
  );
};

export default ImageSourceActions;
