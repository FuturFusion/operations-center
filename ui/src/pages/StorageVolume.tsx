import { useQuery } from "@tanstack/react-query";
import { fetchStorageVolumes } from "api/storage_volume";
import DataTable from "components/DataTable";
import ObjectIncusLink from "components/ObjectIncusLink";
import ProjectIncusLink from "components/ProjectIncusLink";
import { formatDate } from "util/date";

const StorageVolume = () => {
  const {
    data: volumes = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["storage_volumes"],
    queryFn: fetchStorageVolumes,
  });

  if (isLoading) {
    return <div>Loading storage volumes...</div>;
  }

  if (error) {
    return <div>Error while loading storage volumes: {error.message}</div>;
  }

  const headers = [
    "Name",
    "Type",
    "Cluster",
    "Server",
    "Project",
    "Parent name",
    "Last updated",
  ];
  const rows = volumes.map((item) => {
    return [
      {
        content: (
          <ObjectIncusLink
            cluster={item.cluster}
            objectName={item.name}
            incusPath={`/ui/project/${item.project_name}/storage/pool/${item.parent_name}/volumes/custom/${item.name}`}
          />
        ),
        sortKey: item.name,
      },
      {
        content: item.type,
        sortKey: item.type,
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
        content: item.parent_name,
        sortKey: item.parent_name,
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

export default StorageVolume;
