import { useQuery } from "@tanstack/react-query";
import { Badge, Container } from "react-bootstrap";
import { Link, useSearchParams } from "react-router";
import { fetchServers } from "api/server";
import ExtendedDataTable from "components/ExtendedDataTable";
import InventorySearchBox from "components/InventorySearchBox";
import ServerActions from "components/ServerActions";
import ServerStatus from "components/ServerStatus";
import type { Server } from "types/server";
import type { ServerTypeKey } from "util/server";
import { ServerTypeString } from "util/server";
import { BsLink45Deg } from "react-icons/bs";

const Server = () => {
  const [searchParams] = useSearchParams();
  const filter = searchParams.get("filter");

  const sortData = (a: Server, b: Server) => {
    return a.cluster.localeCompare(b.cluster) || a.name.localeCompare(b.name);
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

  const headers = [
    "Name",
    "Description / Properties",
    "Cluster",
    "Type",
    "Status",
    "Actions",
  ];
  const rows = servers.map((item) => {
    return [
      {
        content: [
          <Link
            to={`/ui/provisioning/servers/${item.name}`}
            className="data-table-link"
            title="Server details"
          >
            {item.name}
          </Link>,
          <Link
            to={item.public_connection_url || item.connection_url}
            target="_blank"
            className="data-table-link"
            title="Access server through external URL"
          >
            <BsLink45Deg color="grey" size={25} />
          </Link>,
        ],
        sortKey: item.name,
      },
      {
        content: (
          <>
            {item.description}
            <br />
            {item.properties &&
              Object.entries(item.properties).map(([key, value]) => [
                <Badge bg="primary">
                  {key}:{value}
                </Badge>,
                <span> </span>,
              ])}
          </>
        ),
      },
      {
        content: item.cluster,
        sortKey: item.cluster,
      },
      {
        content: ServerTypeString[item.server_type as ServerTypeKey],
        sortKey: item.server_type,
      },
      {
        content: <ServerStatus server={item} />,
        sortKey: item.server_status,
      },
      {
        content: <ServerActions server={item} />,
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
