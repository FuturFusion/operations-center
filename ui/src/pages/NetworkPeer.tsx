import { Container } from "react-bootstrap";
import { useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { fetchNetworkPeers } from "api/network_peer";
import ClusterLink from "components/ClusterLink";
import ExtendedDataTable from "components/ExtendedDataTable";
import InventorySearchBox from "components/InventorySearchBox";
import { formatDate } from "util/date";

const NetworkPeer = () => {
  const [searchParams] = useSearchParams();
  const filter = searchParams.get("filter");

  const {
    data: peers = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["network_peers", filter],
    queryFn: () => fetchNetworkPeers(filter || ""),
    retry: false,
  });

  const headers = [
    "Name",
    "Type",
    "Network name",
    "Project",
    "Cluster",
    "Last updated",
  ];
  const rows = peers.map((item) => {
    return [
      {
        content: item.name,
        sortKey: item.name,
      },
      {
        content: item.object.type,
        sortKey: item.object.type,
      },
      {
        content: item.parent_name,
        sortKey: item.parent_name,
      },
      {
        content: item.object.target_project,
        sortKey: item.object.target_project,
      },
      {
        content: <ClusterLink cluster={item.cluster} />,
        sortKey: item.cluster,
      },
      {
        content: formatDate(item.last_updated),
        sortKey: item.last_updated,
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

export default NetworkPeer;
