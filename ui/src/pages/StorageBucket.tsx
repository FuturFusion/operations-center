import { useQuery } from "@tanstack/react-query";
import { fetchStorageBuckets } from "api/storage_bucket";
import DataTable from "components/DataTable";

const StorageBucket = () => {
  const {
    data: buckets = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["storage_buckets"],
    queryFn: fetchStorageBuckets,
  });

  if (isLoading) {
    return <div>Loading storage buckets...</div>;
  }

  if (error) {
    return <div>Error while loading storage buckets: {error.message}</div>;
  }

  const headers = [
    "Name",
    "Cluster",
    "Server",
    "Project name",
    "Parent name",
    "Last updated",
  ];
  const rows = buckets.map((item) => {
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
        content: item.parent_name,
        sortKey: item.parent_name,
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

export default StorageBucket;
