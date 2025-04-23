import { useQuery } from "@tanstack/react-query";
import { fetchProjects } from "api/projects";
import DataTable from "components/DataTable";

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

  const headers = ["ID", "Cluster", "Name", "Last updated"];
  const rows = projects.map((item) => {
    return [
      {
        content: item.id,
        sortKey: item.id,
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

export default Project;
