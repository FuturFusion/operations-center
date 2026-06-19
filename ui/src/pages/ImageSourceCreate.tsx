import { useNavigate } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useNotification } from "context/notificationContext";
import { createImageSource } from "api/image_incus_source";
import Breadcrumbs from "components/Breadcrumbs";
import ImageSourceForm from "components/ImageSourceForm";
import type { ImageSource } from "types/image_incus_source";

const ImageSourceCreate = () => {
  const { notify } = useNotification();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const onSubmit = (values: ImageSource) => {
    createImageSource(JSON.stringify(values, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Image source created`);
          queryClient.invalidateQueries({ queryKey: ["image-sources"] });
          navigate("/ui/images-view/sources");
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during image source creation: ${e}`);
      });
  };

  return (
    <div className="d-flex flex-column">
      <Breadcrumbs />
      <div className="scroll-container flex-grow-1 p-3">
        <ImageSourceForm onSubmit={onSubmit} />
      </div>
    </div>
  );
};

export default ImageSourceCreate;
