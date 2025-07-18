import { useQuery } from "@tanstack/react-query";
import { fetchInstances } from "api/instance";
import DataTable from "components/DataTable";

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

  const headers = ["Name", "Cluster", "Server", "Project name", "Last updated"];
  const rows = instances.map((item) => {
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
        content: item.server,
        sortKey: item.server,
      },
      {
        content: item.project_name,
        sortKey: item.project_name,
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

export default Instance;
