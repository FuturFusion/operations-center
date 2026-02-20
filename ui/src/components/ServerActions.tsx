import { FC, useEffect, useState } from "react";
import { MdOutlineSync } from "react-icons/md";
import { useQueryClient } from "@tanstack/react-query";
import { resyncServer } from "api/server";
import ServerEvacuateBtn from "components/ServerEvacuateBtn";
import ServerPoweroffBtn from "components/ServerPoweroffBtn";
import ServerRebootBtn from "components/ServerRebootBtn";
import ServerRestoreBtn from "components/ServerRestoreBtn";
import ServerUpdateBtn from "components/ServerUpdateBtn";
import { useNotification } from "context/notificationContext";
import type { Server } from "types/server";
import { ServerAction, ServerType } from "util/server";

interface Props {
  server: Server;
}

const ServerActions: FC<Props> = ({ server }) => {
  const { notify } = useNotification();
  const queryClient = useQueryClient();
  const [recommendedAction, setRecommendedAction] = useState("");

  const actionStyle = {
    cursor: "pointer",
    color: "grey",
  };

  useEffect(() => {
    let action = "";
    if (server.version_data.needs_update) {
      action = ServerAction.Update;
    } else if (server.version_data.needs_reboot) {
      if (
        !server.version_data.in_maintenance &&
        server.server_type == ServerType.Incus
      ) {
        action = ServerAction.Evacuate;
      } else {
        action = ServerAction.Reboot;
      }
    } else if (server.version_data.in_maintenance) {
      action = ServerAction.Restore;
    }

    setRecommendedAction(action);
  }, [
    server.server_type,
    server.version_data.needs_update,
    server.version_data.needs_reboot,
    server.version_data.in_maintenance,
  ]);

  const onResyncServer = () => {
    resyncServer(server.name)
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Server resync triggered`);
          queryClient.invalidateQueries({ queryKey: ["servers"] });
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during server sync: ${e}`);
      });
  };

  const showButton = (action: string): boolean => {
    const versionData = server.version_data;
    if (versionData.needs_update && action == ServerAction.Update) {
      return true;
    }

    if (action == ServerAction.Reboot) {
      if (
        versionData.needs_update &&
        versionData.in_maintenance &&
        !versionData.needs_reboot
      ) {
        return false;
      }

      return true;
    }

    if (versionData.in_maintenance && action == ServerAction.Restore) {
      return true;
    }

    if (!versionData.in_maintenance && action == ServerAction.Evacuate) {
      return true;
    }

    return false;
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
      {showButton(ServerAction.Reboot) && (
        <ServerRebootBtn
          server={server}
          recommended={recommendedAction == ServerAction.Reboot}
        />
      )}
      {showButton(ServerAction.Restore) && (
        <ServerRestoreBtn
          server={server}
          recommended={recommendedAction == ServerAction.Restore}
        />
      )}
      {showButton(ServerAction.Evacuate) && (
        <ServerEvacuateBtn
          server={server}
          recommended={recommendedAction == ServerAction.Evacuate}
        />
      )}
      {showButton(ServerAction.Update) && (
        <ServerUpdateBtn
          server={server}
          recommended={recommendedAction == ServerAction.Update}
        />
      )}
      <ServerPoweroffBtn server={server} />
    </div>
  );
};

export default ServerActions;
