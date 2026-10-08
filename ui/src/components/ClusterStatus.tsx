import { FC } from "react";
import type { Cluster } from "types/cluster";
import { MdErrorOutline } from "react-icons/md";
import { MdSystemUpdateAlt } from "react-icons/md";
import { MdExitToApp } from "react-icons/md";
import { MdOutlineReplay } from "react-icons/md";
import { ClusterUpdateInProgress } from "util/cluster";

interface Props {
  cluster: Cluster;
}

const ClusterStatus: FC<Props> = ({ cluster }) => {
  const inProgressStatus = cluster.update_status?.in_progress_status;
  const inProgress = inProgressStatus?.in_progress ?? "";
  const isError = inProgress == ClusterUpdateInProgress.Error;

  return (
    <div>
      {cluster.status}
      {inProgress != "" && (
        <span className={isError ? "text-danger" : undefined}>
          {" ("}
          {inProgress}
          {")"}
        </span>
      )}{" "}
      {isError && (
        <MdErrorOutline
          className="text-danger"
          size={25}
          title="The cluster update failed"
        />
      )}
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
      {inProgressStatus?.status_description && (
        <>
          <br />
          <span
            className={isError ? "text-danger" : undefined}
            style={isError ? undefined : { color: "var(--bs-secondary-color)" }}
          >
            {inProgressStatus.status_description}
          </span>
        </>
      )}
    </div>
  );
};

export default ClusterStatus;
