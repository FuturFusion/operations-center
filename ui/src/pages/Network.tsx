import { Container } from "react-bootstrap";
import { useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { fetchNetworks } from "api/network";
import ExtendedDataTable from "components/ExtendedDataTable";
import InventorySearchBox from "components/InventorySearchBox";
import ClusterLink from "components/ClusterLink";
import ObjectIncusLink from "components/ObjectIncusLink";
import ProjectIncusLink from "components/ProjectIncusLink";
import type { Network } from "types/network";
import { formatDate } from "util/date";

const Network = () => {
  const [searchParams] = useSearchParams();
  const filter = searchParams.get("filter");

  const sortData = (a: Network, b: Network) => {
    return (
      a.name.localeCompare(b.name) ||
      a.project_name.localeCompare(b.project_name) ||
      a.cluster.localeCompare(b.cluster)
    );
  };

  const {
    data: networks = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["networks", filter],
    queryFn: () => fetchNetworks(filter || ""),
    select: (items) => [...items].sort(sortData),
    retry: false,
  });

  const headers = ["Name", "Type", "Project", "Cluster", "Last updated"];
  const rows = networks.map((item) => {
    return [
      {
        content: (
          <ObjectIncusLink
            cluster={item.cluster}
            objectName={item.name}
            incusPath={`/ui/project/${item.project_name}/network/${item.name}`}
          />
        ),
        sortKey: item.name,
      },
      {
        content: item.object.type,
        sortKey: item.object.type,
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

export default Network;
