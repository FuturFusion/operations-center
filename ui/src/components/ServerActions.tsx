import { FC } from "react";
import { MdOutlineSync } from "react-icons/md";
import { resyncServer } from "api/server";
import ServerPoweroffBtn from "components/ServerPoweroffBtn";
import ServerRebootBtn from "components/ServerRebootBtn";
import { useNotification } from "context/notificationContext";
import { Server } from "types/server";

interface Props {
  server: Server;
}

const ServerActions: FC<Props> = ({ server }) => {
  const { notify } = useNotification();
  const actionStyle = {
    cursor: "pointer",
    color: "grey",
  };

  const onResyncServer = () => {
    resyncServer(server.name)
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Server resync triggered`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during server sync: ${e}`);
      });
  };

  return (
    <div>
      <MdOutlineSync
        size={25}
        title="Resync server's state"
        style={actionStyle}
        onClick={() => {
          onResyncServer();
        }}
      />
      <ServerRebootBtn server={server} />
      <ServerPoweroffBtn server={server} />
    </div>
  );
};

export default ServerActions;
