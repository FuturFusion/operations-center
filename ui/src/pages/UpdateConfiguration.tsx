import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import { fetchUpdate, updateUpdate } from "api/update";
import UpdateForm from "components/UpdateForm";
import { useNotification } from "context/notificationContext";
import { UpdateFormValues } from "types/update";

const UpdateConfiguration = () => {
  const { uuid } = useParams() as { uuid: string };
  const { notify } = useNotification();
  const navigate = useNavigate();

  const onSubmit = (values: UpdateFormValues) => {
    updateUpdate(uuid, JSON.stringify(values, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Update ${uuid} updated`);
          navigate(`/ui/provisioning/updates/${uuid}/configuration`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during Update update: ${e}`);
      });
  };

  const {
    data: update = undefined,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["updates", uuid],
    queryFn: () => fetchUpdate(uuid),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading update</div>;
  }

  return <UpdateForm update={update} onSubmit={onSubmit} />;
};

export default UpdateConfiguration;
