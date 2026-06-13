import { FC, useState } from "react";
import Button from "react-bootstrap/Button";
import { BsTrash } from "react-icons/bs";
import { useQueryClient } from "@tanstack/react-query";
import { deleteIncusImage } from "api/image_incus";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { IncusImage } from "types/image_incus";

interface Props {
  image: IncusImage;
}

const IncusImageActions: FC<Props> = ({ image }) => {
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const queryClient = useQueryClient();
  const { notify } = useNotification();

  const actionStyle = {
    cursor: "pointer",
    color: "grey",
  };

  const handleDelete = () => {
    setShowDeleteModal(false);

    deleteIncusImage(image.name)
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Image ${image.name} deleted`);
          queryClient.invalidateQueries({ queryKey: ["incus-images"] });
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during image deletion: ${e}`);
      });
  };

  return (
    <div>
      <BsTrash
        title="Delete"
        style={actionStyle}
        onClick={() => setShowDeleteModal(true)}
      />
      <ModalWindow
        show={showDeleteModal}
        handleClose={() => setShowDeleteModal(false)}
        title="Delete image?"
        footer={
          <>
            <Button variant="danger" onClick={handleDelete}>
              Delete
            </Button>
          </>
        }
      >
        <p>
          Are you sure you want to delete the image "{image.name}"?
          <br />
          This action cannot be undone.
        </p>
      </ModalWindow>
    </div>
  );
};

export default IncusImageActions;
