import { useQuery } from "@tanstack/react-query";
import { fetchNetworkZones } from "api/network_zone";
import DataTable from "components/DataTable";
import ProjectIncusLink from "components/ProjectIncusLink";
import { formatDate } from "util/date";

const NetworkZone = () => {
  const {
    data: zones = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["network_zones"],
    queryFn: fetchNetworkZones,
  });

  if (isLoading) {
    return <div>Loading network zones...</div>;
  }

  if (error) {
    return <div>Error while loading network zones: {error.message}</div>;
  }

  const headers = ["Name", "Cluster", "Project", "Last updated"];
  const rows = zones.map((item) => {
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

export default NetworkZone;
