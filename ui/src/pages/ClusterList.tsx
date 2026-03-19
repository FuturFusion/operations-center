import { Badge, Button, Container } from "react-bootstrap";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useSearchParams } from "react-router";
import { fetchClusters } from "api/cluster";
import ClusterActions from "components/ClusterActions";
import ClusterStatus from "components/ClusterStatus";
import ExtendedDataTable from "components/ExtendedDataTable";
import InventorySearchBox from "components/InventorySearchBox";
import type { Cluster } from "types/cluster";
import { BsLink45Deg } from "react-icons/bs";

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

  const headers = ["Name", "Description / Properties", "Status", "Actions"];
  const rows = clusters.map((item) => {
    return [
      {
        content: [
          <Link
            to={`/ui/provisioning/clusters/${item.name}`}
            className="data-table-link"
            title="Cluster details"
          >
            {item.name}
          </Link>,
          <Link
            to={item.connection_url}
            target="_blank"
            className="data-table-link"
            title="Access server through external URL"
          >
            <BsLink45Deg color="grey" size={25} />
          </Link>,
        ],
        sortKey: item.name,
      },
      {
        content: (
          <>
            {item.description}
            <br />
            {item.properties &&
              Object.entries(item.properties).map(([key, value]) => [
                <Badge bg="primary">
                  {key}:{value}
                </Badge>,
                <span> </span>,
              ])}
          </>
        ),
      },
      {
        content: <ClusterStatus cluster={item} />,
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
