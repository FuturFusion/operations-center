import { useQuery } from "@tanstack/react-query";
import { fetchNetworks } from "api/network";
import DataTable from "components/DataTable";

const Network = () => {
  const {
    data: networks = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["networks"],
    queryFn: fetchNetworks,
  });

  if (isLoading) {
    return <div>Loading networks...</div>;
  }

  if (error) {
    return <div>Error while loading networks: {error.message}</div>;
  }

  const headers = ["UUID", "Cluster", "Project name", "Name", "Last updated"];
  const rows = networks.map((item) => {
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
        content: item.project_name,
        sortKey: item.project_name,
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

export default Network;
