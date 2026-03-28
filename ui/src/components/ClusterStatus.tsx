import { FC } from "react";
import type { Cluster } from "types/cluster";
import { MdSystemUpdateAlt } from "react-icons/md";
import { MdExitToApp } from "react-icons/md";
import { MdOutlineReplay } from "react-icons/md";

interface Props {
  cluster: Cluster;
}

const ClusterStatus: FC<Props> = ({ cluster }) => {
  return (
    <div>
      {cluster.status}
      {cluster.update_status?.in_progress_status?.in_progress != "" && (
        <>
          {" ("}
          {cluster.update_status?.in_progress_status?.in_progress}
          {")"}
        </>
      )}{" "}
      {cluster.update_status?.in_maintenance?.length > 0 && (
        <MdExitToApp
          color="orange"
          size={25}
          title="One or more servers are in maintenance"
        />
      )}
      {cluster.update_status?.needs_update?.length > 0 && (
        <MdSystemUpdateAlt
          color="orange"
          size={25}
          title="One or more servers have pending updates"
        />
      )}
      {cluster.update_status?.needs_reboot?.length > 0 && (
        <MdOutlineReplay
          color="orange"
          size={25}
          title="One or more servers require a reboot"
        />
      )}
      {cluster.update_status?.in_progress_status?.status_description != "" && (
        <>
          <br />
          <span style={{ color: "#00000090" }}>
            {cluster.update_status.in_progress_status.status_description}
          </span>
        </>
      )}
    </div>
  );
};

export default ClusterStatus;
