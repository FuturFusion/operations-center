import { FC } from "react";
import { Link } from "react-router";

type Props = {
  server: string;
  displayName?: string;
};

const ServerLink: FC<Props> = ({ server, displayName }) => {
  return (
    <Link to={`/ui/provisioning/servers/${server}`} className="data-table-link">
      {displayName ? displayName : server}
    </Link>
  );
};

export default ServerLink;
