import { Container } from "react-bootstrap";
import { useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { fetchNetworkZones } from "api/network_zone";
import ClusterLink from "components/ClusterLink";
import ExtendedDataTable from "components/ExtendedDataTable";
import InventorySearchBox from "components/InventorySearchBox";
import ProjectIncusLink from "components/ProjectIncusLink";
import type { NetworkZone } from "types/network_zone";
import { formatDate } from "util/date";

const NetworkZone = () => {
  const [searchParams] = useSearchParams();
  const filter = searchParams.get("filter");

  const sortData = (a: NetworkZone, b: NetworkZone) => {
    return (
      a.name.localeCompare(b.name) ||
      a.project_name.localeCompare(b.project_name) ||
      a.cluster.localeCompare(b.cluster)
    );
  };

  const {
    data: zones = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["network_zones", filter],
    queryFn: () => fetchNetworkZones(filter || ""),
    select: (items) => [...items].sort(sortData),
    retry: false,
  });

  const headers = ["Name", "Project", "Cluster", "Last updated"];
  const rows = zones.map((item) => {
    return [
      {
        content: item.name,
        sortKey: item.name,
      },
      {
        content: (
          <ProjectIncusLink
            cluster={item.cluster}
            project={item.project_name}
          />
        ),
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

export default NetworkZone;
