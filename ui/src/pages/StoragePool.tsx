import { Container } from "react-bootstrap";
import { useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { fetchStoragePools } from "api/storage_pool";
import ClusterLink from "components/ClusterLink";
import ExtendedDataTable from "components/ExtendedDataTable";
import InventorySearchBox from "components/InventorySearchBox";
import ObjectIncusLink from "components/ObjectIncusLink";
import type { StoragePool } from "types/storage_pool";
import { formatDate } from "util/date";

const StoragePool = () => {
  const [searchParams] = useSearchParams();
  const filter = searchParams.get("filter");

  const sortData = (a: StoragePool, b: StoragePool) => {
    return a.name.localeCompare(b.name) || a.cluster.localeCompare(b.cluster);
  };

  const {
    data: pools = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["storage_pools", filter],
    queryFn: () => fetchStoragePools(filter || ""),
    select: (items) => [...items].sort(sortData),
    retry: false,
  });

  const headers = ["Name", "Driver", "Cluster", "Last updated"];
  const rows = pools.map((item) => {
    return [
      {
        content: (
          <ObjectIncusLink
            cluster={item.cluster}
            objectName={item.name}
            incusPath={`/ui/project/default/storage/pool/${item.name}`}
          />
        ),
        sortKey: item.name,
      },
      {
        content: item.object.driver,
        sortKey: item.object.driver,
      },
      {
        content: <ClusterLink cluster={item.cluster} />,
        sortKey: item.cluster,
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

export default StoragePool;
