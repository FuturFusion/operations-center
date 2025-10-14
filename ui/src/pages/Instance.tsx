import { Container } from "react-bootstrap";
import { useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { fetchInstances } from "api/instance";
import ExtendedDataTable from "components/ExtendedDataTable";
import InventorySearchBox from "components/InventorySearchBox";
import ObjectIncusLink from "components/ObjectIncusLink";
import ProjectIncusLink from "components/ProjectIncusLink";
import { formatDate } from "util/date";

const Instance = () => {
  const [searchParams] = useSearchParams();
  const filter = searchParams.get("filter");

  const {
    data: instances = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["instances", filter],
    queryFn: () => fetchInstances(filter || ""),
    retry: false,
  });

  const headers = ["Name", "Cluster", "Server", "Project", "Last updated"];
  const rows = instances.map((item) => {
    return [
      {
        content: (
          <ObjectIncusLink
            cluster={item.cluster}
            objectName={item.name}
            incusPath={`/ui/project/${item.project_name}/instance/${item.name}`}
          />
        ),
        sortKey: item.name,
      },
      {
        content: item.cluster,
        sortKey: item.cluster,
      },
      {
        content: item.server,
        sortKey: item.server,
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

export default Instance;
