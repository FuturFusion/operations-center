import { Container } from "react-bootstrap";
import { useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { fetchStorageVolumes } from "api/storage_volume";
import ExtendedDataTable from "components/ExtendedDataTable";
import InventorySearchBox from "components/InventorySearchBox";
import ObjectIncusLink from "components/ObjectIncusLink";
import ProjectIncusLink from "components/ProjectIncusLink";
import { formatDate } from "util/date";

const StorageVolume = () => {
  const [searchParams] = useSearchParams();
  const filter = searchParams.get("filter");

  const {
    data: volumes = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["storage_volumes", filter],
    queryFn: () => fetchStorageVolumes(filter || ""),
    retry: false,
  });

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

export default StorageVolume;
