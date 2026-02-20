import { useNavigate } from "react-router";
import { useNotification } from "context/notificationContext";
import { createChannel } from "api/channel";
import ChannelForm from "components/ChannelForm";
import type { Channel } from "types/channel";

const ChannelCreate = () => {
  const { notify } = useNotification();
  const navigate = useNavigate();

  const onSubmit = (values: Channel) => {
    createChannel(JSON.stringify(values, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Channel created`);
          navigate("/ui/provisioning/updates-view/channels");
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during channel creation: ${e}`);
      });
  };

  return (
    <div className="d-flex flex-column">
      <div className="scroll-container flex-grow-1 p-3">
        <ChannelForm onSubmit={onSubmit} />
      </div>
    </div>
  );
};

export default ChannelCreate;
