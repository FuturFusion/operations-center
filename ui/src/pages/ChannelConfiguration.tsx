import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import { fetchChannel, updateChannel } from "api/channel";
import ChannelForm from "components/ChannelForm";
import { useNotification } from "context/notificationContext";
import { Channel } from "types/channel";

const ChannelConfiguration = () => {
  const { name } = useParams() as { name: string };
  const { notify } = useNotification();
  const navigate = useNavigate();

  const onSubmit = (values: Channel) => {
    updateChannel(name, JSON.stringify(values, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Channel ${name} updated`);
          navigate(`/ui/provisioning/channels/${name}/configuration`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during channel update: ${e}`);
      });
  };

  const {
    data: channel = undefined,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["channels", name],
    queryFn: () => fetchChannel(name),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading channel</div>;
  }

  return <ChannelForm channel={channel} onSubmit={onSubmit} />;
};

export default ChannelConfiguration;
