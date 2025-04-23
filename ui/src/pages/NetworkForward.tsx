import { useQuery } from "@tanstack/react-query";
import { fetchNetworkForwards } from "api/networks";
import DataTable from "components/DataTable";

const NetworkForward = () => {
  const {
    data: network_forwards = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["network_forwards"],
    queryFn: fetchNetworkForwards,
  });

  if (isLoading) {
    return <div>Loading network forwards...</div>;
  }

  if (error) {
    return <div>Error while loading network forwards: {error.message}</div>;
  }

  const headers = ["ID", "Cluster", "Network name", "Name", "Last updated"];
  const rows = network_forwards.map((item) => {
    return [
      {
        content: item.id,
        sortKey: item.id,
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

export default NetworkForward;
