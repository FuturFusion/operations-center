import { FC } from "react";
import { MdWarning } from "react-icons/md";
import type { Server } from "types/server";

interface Props {
  server: Server;
}

const ServerStatus: FC<Props> = ({ server }) => {
  return (
    <div>
      {server.server_status}
      {server.server_status_detail != "" && (
        <>
          {" ("}
          {server.server_status_detail}
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
