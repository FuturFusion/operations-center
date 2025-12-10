import { useQuery } from "@tanstack/react-query";
import { Container } from "react-bootstrap";
import { Link, useSearchParams } from "react-router";
import { fetchServers } from "api/server";
import ExtendedDataTable from "components/ExtendedDataTable";
import InventorySearchBox from "components/InventorySearchBox";
import type { Server } from "types/server";
import type { ServerTypeKey } from "util/server";
import { ServerTypeString } from "util/server";

const Server = () => {
  const [searchParams] = useSearchParams();
  const filter = searchParams.get("filter");

  const sortData = (a: Server, b: Server) => {
    return a.name.localeCompare(b.name) || a.cluster.localeCompare(b.cluster);
  };

  const {
    data: servers = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["servers", filter],
    queryFn: () => fetchServers(filter || ""),
    select: (items) => [...items].sort(sortData),
    retry: false,
  });

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
      <Container className="d-flex justify-content-center">
        <InventorySearchBox />
      </Container>
      <div className="d-flex flex-column">
        <div className="scroll-container flex-grow-1">
          <ExtendedDataTable
            headers={headers}
            rows={rows}
            isLoading={isLoading}
            error={error}
          />
        </div>
      </div>
    </>
  );
};

export default Server;
