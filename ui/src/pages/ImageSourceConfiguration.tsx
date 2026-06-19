import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import { fetchImageSource, updateImageSource } from "api/image_incus_source";
import ImageSourceForm from "components/ImageSourceForm";
import { useNotification } from "context/notificationContext";
import { ImageSource } from "types/image_incus_source";

const ImageSourceConfiguration = () => {
  const { name } = useParams() as { name: string };
  const { notify } = useNotification();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const onSubmit = (values: ImageSource) => {
    updateImageSource(name, JSON.stringify(values, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Image source ${name} updated`);
          queryClient.invalidateQueries({ queryKey: ["image-sources"] });
          navigate(`/ui/images/sources/${name}/configuration`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during image source update: ${e}`);
      });
  };

  const {
    data: source = undefined,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["image-sources", name],
    queryFn: () => fetchImageSource(name),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading image source</div>;
  }

  return <ImageSourceForm source={source} onSubmit={onSubmit} />;
};

export default ImageSourceConfiguration;
