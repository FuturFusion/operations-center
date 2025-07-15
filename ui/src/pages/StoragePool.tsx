import { useQuery } from "@tanstack/react-query";
import { fetchStoragePools } from "api/storage_pool";
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

  const headers = ["Name", "Cluster", "Last updated"];
  const rows = pools.map((item) => {
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
