import { useQuery } from "@tanstack/react-query";
import { fetchProjects } from "api/project";
import DataTable from "components/DataTable";
import ProjectIncusLink from "components/ProjectIncusLink";
import { formatDate } from "util/date";

const Project = () => {
  const {
    data: projects = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["projects"],
    queryFn: fetchProjects,
  });

  if (isLoading) {
    return <div>Loading projects...</div>;
  }

  if (error) {
    return <div>Error while loading projects: {error.message}</div>;
  }

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
      <div className="d-flex flex-column">
        <div className="scroll-container flex-grow-1">
          <DataTable headers={headers} rows={rows} />
        </div>
      </div>
    </>
  );
};

export default Project;
