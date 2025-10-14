import { Container } from "react-bootstrap";
import { useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { fetchStorageBuckets } from "api/storage_bucket";
import ExtendedDataTable from "components/ExtendedDataTable";
import InventorySearchBox from "components/InventorySearchBox";
import ObjectIncusLink from "components/ObjectIncusLink";
import ProjectIncusLink from "components/ProjectIncusLink";
import { formatDate } from "util/date";

const StorageBucket = () => {
  const [searchParams] = useSearchParams();
  const filter = searchParams.get("filter");

  const {
    data: buckets = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["storage_buckets", filter],
    queryFn: () => fetchStorageBuckets(filter || ""),
    retry: false,
  });

  const headers = [
    "Name",
    "Cluster",
    "Server",
    "Project",
    "Parent name",
    "Last updated",
  ];
  const rows = buckets.map((item) => {
    return [
      {
        content: (
          <ObjectIncusLink
            cluster={item.cluster}
            objectName={item.name}
            incusPath={`/ui/project/${item.project_name}/storage-pools/${item.parent_name}/buckets/${item.name}`}
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
      <Container className="d-flex justify-content-center">
        <InventorySearchBox />
      </Container>
      <div className="d-flex flex-column">
        <div className="scroll-container flex-grow-1">
          <ExtendedDataTable
            headers={headers}
            rows={rows}
            isLoading={isLoading}
            error={error}
          />
        </div>
      </div>
    </>
  );
};

export default StorageBucket;
