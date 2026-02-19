import { Container } from "react-bootstrap";
import { useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { fetchNetworkLoadBalancers } from "api/network_load_balancer";
import ClusterLink from "components/ClusterLink";
import ExtendedDataTable from "components/ExtendedDataTable";
import InventorySearchBox from "components/InventorySearchBox";
import type { NetworkLoadBalancer } from "types/network_load_balancer";
import { formatDate } from "util/date";

const NetworkLoadBalancer = () => {
  const [searchParams] = useSearchParams();
  const filter = searchParams.get("filter");

  const sortData = (a: NetworkLoadBalancer, b: NetworkLoadBalancer) => {
    return (
      a.cluster.localeCompare(b.cluster) ||
      a.project_name.localeCompare(b.project_name) ||
      a.parent_name.localeCompare(b.parent_name) ||
      a.name.localeCompare(b.name)
    );
  };

  const {
    data: load_balancers = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["network_load_balancers", filter],
    queryFn: () => fetchNetworkLoadBalancers(filter || ""),
    select: (items) => [...items].sort(sortData),
    retry: false,
  });

  const headers = [
    "Address",
    "Network name",
    "Project",
    "Cluster",
    "Last updated",
  ];
  const rows = load_balancers.map((item) => {
    return [
      {
        content: item.name,
        sortKey: item.name,
      },
      {
        content: item.parent_name,
        sortKey: item.parent_name,
      },
      {
        content: item.project_name,
        sortKey: item.project_name,
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

export default NetworkLoadBalancer;
