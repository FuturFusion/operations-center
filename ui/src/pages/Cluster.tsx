import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import { fetchClusters } from "api/cluster";
import ClusterActions from "components/ClusterActions";
import DataTable from "components/DataTable";
import { formatDate } from "util/date";

const Cluster = () => {
  const {
    data: clusters = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["clusters"],
    queryFn: fetchClusters,
  });

  if (isLoading) {
    return <div>Loading clusters...</div>;
  }

  if (error) {
    return <div>Error while loading clusters: {error.message}</div>;
  }

  const headers = [
    "Name",
    "Connection URL",
    "Status",
    "Last updated",
    "Actions",
  ];
  const rows = clusters.map((item) => {
    return [
      {
        content: item.name,
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
        content: formatDate(item.last_updated || ""),
        sortKey: item.last_updated,
      },
      {
        content: <ClusterActions cluster={item} />,
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

export default Cluster;
