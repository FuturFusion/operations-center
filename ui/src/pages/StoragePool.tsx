import { useQuery } from "@tanstack/react-query";
import { fetchStoragePools } from "api/storages";
import DataTable from "components/DataTable";

const StoragePool = () => {
  const {
    data: pools = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["storage_pools"],
    queryFn: fetchStoragePools,
  });

  if (isLoading) {
    return <div>Loading storage pools...</div>;
  }

  if (error) {
    return <div>Error while loading storage pools: {error.message}</div>;
  }

  const headers = ["UUID", "Cluster", "Name", "Last updated"];
  const rows = pools.map((item) => {
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

export default StoragePool;
