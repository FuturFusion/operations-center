import { Container } from "react-bootstrap";
import { useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { fetchProjects } from "api/project";
import ExtendedDataTable from "components/ExtendedDataTable";
import InventorySearchBox from "components/InventorySearchBox";
import ProjectIncusLink from "components/ProjectIncusLink";
import { formatDate } from "util/date";

const Project = () => {
  const [searchParams] = useSearchParams();
  const filter = searchParams.get("filter");

  const {
    data: projects = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["projects", filter],
    queryFn: () => fetchProjects(filter || ""),
    retry: false,
  });

  const headers = ["Name", "Cluster", "Last updated"];
  const rows = projects.map((item) => {
    return [
      {
        content: (
          <ProjectIncusLink cluster={item.cluster} project={item.name} />
        ),
        sortKey: item.name,
      },
      {
        content: item.cluster,
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

export default Project;
