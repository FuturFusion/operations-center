import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import { fetchIncusImage, updateIncusImage } from "api/image_incus";
import IncusImageForm from "components/IncusImageForm";
import { useNotification } from "context/notificationContext";
import { IncusImageFormValues } from "types/image_incus";

const IncusImageConfiguration = () => {
  const { name } = useParams() as { name: string };
  const { notify } = useNotification();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const onSubmit = (values: IncusImageFormValues) => {
    updateIncusImage(name, JSON.stringify(values, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Image ${name} updated`);
          queryClient.invalidateQueries({ queryKey: ["incus-images"] });
          navigate(`/ui/images/incus/${name}/configuration`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during image update: ${e}`);
      });
  };

  const {
    data: image = undefined,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["incus-images", name],
    queryFn: () => fetchIncusImage(name),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading image</div>;
  }

  return <IncusImageForm image={image} onSubmit={onSubmit} />;
};

export default IncusImageConfiguration;
