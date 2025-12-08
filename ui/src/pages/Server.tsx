import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import { fetchServers } from "api/server";
import DataTable from "components/DataTable";
import type { ServerTypeKey } from "util/server";
import { ServerTypeString } from "util/server";

const Server = () => {
  const {
    data: servers = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["servers"],
    queryFn: () => fetchServers(""),
  });

  if (isLoading) {
    return <div>Loading servers...</div>;
  }

  if (error) {
    return <div>Error while loading servers: {error.message}</div>;
  }

  const headers = ["Name", "Cluster", "Connection URL", "Type", "Status"];
  const rows = servers.map((item) => {
    return [
      {
        content: (
          <Link
            to={`/ui/provisioning/servers/${item.name}`}
            className="data-table-link"
          >
            {item.name}
          </Link>
        ),
        sortKey: item.name,
      },
      {
        content: item.cluster,
        sortKey: item.cluster,
      },
      {
        content: (
          <Link
            to={`${item.connection_url}`}
            target="_blank"
            className="data-table-link"
          >
            {item.connection_url}
          </Link>
        ),
        sortKey: item.connection_url,
      },
      {
        content: ServerTypeString[item.server_type as ServerTypeKey],
        sortKey: item.server_type,
      },
      {
        content: item.server_status,
        sortKey: item.server_status,
      },
    ];
  });

  return (
    <>
      <div className="d-flex flex-column">
        <div className="scroll-container flex-grow-1">
          <DataTable headers={headers} rows={rows} />
        </div>
      </div>
    </>
  );
};

export default Server;
