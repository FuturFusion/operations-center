import { Container } from "react-bootstrap";
import { useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { fetchNetworkLoadBalancers } from "api/network_load_balancer";
import ExtendedDataTable from "components/ExtendedDataTable";
import InventorySearchBox from "components/InventorySearchBox";
import { formatDate } from "util/date";

const NetworkLoadBalancer = () => {
  const [searchParams] = useSearchParams();
  const filter = searchParams.get("filter");

  const {
    data: load_balancers = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["network_load_balancers", filter],
    queryFn: () => fetchNetworkLoadBalancers(filter || ""),
    retry: false,
  });

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

export default NetworkLoadBalancer;
