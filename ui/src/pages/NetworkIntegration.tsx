import { useQuery } from "@tanstack/react-query";
import { fetchNetworkIntegrations } from "api/networks";
import DataTable from "components/DataTable";

const NetworkIntegration = () => {
  const {
    data: network_integrations = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["network_integrations"],
    queryFn: fetchNetworkIntegrations,
  });

  if (isLoading) {
    return <div>Loading network integrations...</div>;
  }

  if (error) {
    return <div>Error while loading network integrations: {error.message}</div>;
  }

  const headers = ["UUID", "Cluster", "Name", "Last updated"];
  const rows = network_integrations.map((item) => {
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

export default NetworkIntegration;
