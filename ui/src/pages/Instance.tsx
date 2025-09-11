import { useQuery } from "@tanstack/react-query";
import { fetchInstances } from "api/instance";
import DataTable from "components/DataTable";
import ObjectIncusLink from "components/ObjectIncusLink";
import ProjectIncusLink from "components/ProjectIncusLink";
import { formatDate } from "util/date";

const Instance = () => {
  const {
    data: instances = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["images"],
    queryFn: fetchInstances,
  });

  if (isLoading) {
    return <div>Loading instances...</div>;
  }

  if (error) {
    return <div>Error while loading instances: {error.message}</div>;
  }

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
      <div className="d-flex flex-column">
        <div className="scroll-container flex-grow-1">
          <DataTable headers={headers} rows={rows} />
        </div>
      </div>
    </>
  );
};

export default Instance;
