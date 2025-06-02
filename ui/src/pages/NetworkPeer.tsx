import { useQuery } from "@tanstack/react-query";
import { fetchNetworkPeers } from "api/network_peer";
import DataTable from "components/DataTable";

const NetworkPeer = () => {
  const {
    data: peers = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["network_peers"],
    queryFn: fetchNetworkPeers,
  });

  if (isLoading) {
    return <div>Loading network peers...</div>;
  }

  if (error) {
    return <div>Error while loading network peers: {error.message}</div>;
  }

  const headers = ["UUID", "Cluster", "Network name", "Name", "Last updated"];
  const rows = peers.map((item) => {
    return [
      {
        content: item.uuid,
        sortKey: item.uuid,
      },
      {
        content: item.cluster,
        sortKey: item.cluster,
      },
      {
        content: item.parent_name,
        sortKey: item.parent_name,
      },
      {
        content: item.name,
        sortKey: item.name,
      },
      {
        content: item.last_updated,
        sortKey: item.last_updated,
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

export default NetworkPeer;
