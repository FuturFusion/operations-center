import { Button, Container } from "react-bootstrap";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useSearchParams } from "react-router";
import { fetchClusters } from "api/cluster";
import ClusterActions from "components/ClusterActions";
import ExtendedDataTable from "components/ExtendedDataTable";
import InventorySearchBox from "components/InventorySearchBox";
import type { Cluster } from "types/cluster";

const Cluster = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const filter = searchParams.get("filter");

  const sortData = (a: Cluster, b: Cluster) => {
    return a.name.localeCompare(b.name);
  };

  const {
    data: clusters = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["clusters", filter],
    queryFn: () => fetchClusters(filter || ""),
    select: (items) => [...items].sort(sortData),
    retry: false,
  });

  const headers = ["Name", "Connection URL", "Status", "Actions"];
  const rows = clusters.map((item) => {
    return [
      {
        content: (
          <Link
            to={`/ui/provisioning/clusters/${item.name}`}
            className="data-table-link"
          >
            {item.name}
          </Link>
        ),
        sortKey: item.name,
      },
      {
        content: (
          <Link
            to={`${item.connection_url}`}
            target="_blank"
            className="data-table-link"
          >
            {item.connection_url}
          </Link>
        ),
        sortKey: item.connection_url,
      },
      {
        content: item.status,
        sortKey: item.status,
      },
      {
        content: <ClusterActions cluster={item} />,
      },
    ];
  });

  return (
    <>
      <div className="d-flex flex-column">
        <div className="mx-2 mx-md-4">
          <div className="row">
            <div className="col-12">
              <Container className="d-flex justify-content-center">
                <InventorySearchBox />
              </Container>
            </div>
          </div>
          <div className="row">
            <div className="col-12">
              <Button
                variant="success"
                className="float-end"
                onClick={() => navigate("/ui/provisioning/clusters/create")}
              >
                Create cluster
              </Button>
            </div>
          </div>
        </div>
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

export default Cluster;
