import { Container } from "react-bootstrap";
import { useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { fetchNetworkForwards } from "api/network_forward";
import ClusterLink from "components/ClusterLink";
import ExtendedDataTable from "components/ExtendedDataTable";
import InventorySearchBox from "components/InventorySearchBox";
import ObjectIncusLink from "components/ObjectIncusLink";
import { useNetworkMap } from "context/useNetworks";
import type { NetworkForward } from "types/network_forward";
import { formatDate } from "util/date";

const NetworkForward = () => {
  const [searchParams] = useSearchParams();
  const filter = searchParams.get("filter");
  const { networkMap } = useNetworkMap();

  const sortData = (a: NetworkForward, b: NetworkForward) => {
    return (
      a.name.localeCompare(b.name) ||
      a.project_name.localeCompare(b.project_name) ||
      a.cluster.localeCompare(b.cluster)
    );
  };

  const {
    data: network_forwards = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["network_forwards", filter],
    queryFn: () => fetchNetworkForwards(filter || ""),
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
  const rows = network_forwards.map((item) => {
    return [
      {
        content: (
          <ObjectIncusLink
            cluster={item.cluster}
            objectName={item.name}
            incusPath={`/ui/project/default/network/${item.parent_name}/forwards`}
          />
        ),
        sortKey: item.name,
      },
      {
        content: item.parent_name,
        sortKey: item.parent_name,
      },
      {
        content: networkMap[item.parent_name]?.project_name,
        sortKey: networkMap[item.parent_name]?.project_name,
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

export default NetworkForward;
