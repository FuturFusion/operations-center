import { FC } from "react";
import { Link } from "react-router";

type Props = {
  cluster: string;
  displayName?: string;
};

const ClusterLink: FC<Props> = ({ cluster, displayName }) => {
  return (
    <Link
      to={`/ui/provisioning/clusters/${cluster}`}
      className="data-table-link"
    >
      {displayName ? displayName : cluster}
    </Link>
  );
};

export default ClusterLink;
