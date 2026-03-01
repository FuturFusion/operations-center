import { FC } from "react";
import { MdWarning } from "react-icons/md";
import type { Server } from "types/server";

interface Props {
  server: Server;
}

const ServerStatus: FC<Props> = ({ server }) => {
  let detail = server.server_status_detail;

  if (detail == "") {
    if (server.version_data.in_maintenance == 1) {
      detail = "evacuating";
    } else if (server.version_data.in_maintenance == 2) {
      detail = "evacuated";
    } else if (server.version_data.in_maintenance == 3) {
      detail = "restoring";
    }
  }

  return (
    <div>
      {server.server_status}
      {detail != "" && (
        <>
          {" ("}
          {detail}
          {")"}
        </>
      )}{" "}
      {!server.system_state_is_trusted && (
        <MdWarning
          size={25}
          color="orange"
          title="Server has degraded boot security"
        />
      )}
    </div>
  );
};

export default ServerStatus;
