import { useQuery } from "@tanstack/react-query";
import { fetchNetworkLoadBalancers } from "api/network_load_balancer";
import DataTable from "components/DataTable";

const NetworkLoadBalancer = () => {
  const {
    data: load_balancers = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["network_load_balancers"],
    queryFn: fetchNetworkLoadBalancers,
  });

  if (isLoading) {
    return <div>Loading network load balancers...</div>;
  }

  if (error) {
    return (
      <div>Error while loading network load balancers: {error.message}</div>
    );
  }

  const headers = ["Name", "Cluster", "Network name", "Last updated"];
  const rows = load_balancers.map((item) => {
    return [
      {
        content: item.name,
        sortKey: item.name,
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

export default NetworkLoadBalancer;
