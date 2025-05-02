import { useQuery } from "@tanstack/react-query";
import { fetchStorageVolumes } from "api/storages";
import DataTable from "components/DataTable";

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
    "UUID",
    "Cluster",
    "Server",
    "Project name",
    "Parent name",
    "Name",
    "Last updated",
  ];
  const rows = volumes.map((item) => {
    return [
      {
        content: item.uuid,
        sortKey: item.uuid,
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
        content: item.parent_name,
        sortKey: item.parent_name,
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

export default StorageVolume;
