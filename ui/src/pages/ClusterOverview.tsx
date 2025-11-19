import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router";
import { fetchCluster } from "api/cluster";
import { formatDate } from "util/date";

const ClusterOverview = () => {
  const { name } = useParams();

  const {
    data: cluster = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["clusters", name],
    queryFn: () => fetchCluster(name || ""),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading cluster</div>;
  }

  return (
    <div className="container">
      <div className="row">
        <div className="col-2 detail-table-header">Name</div>
        <div className="col-10 detail-table-cell">{cluster?.name}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Connection URL</div>
        <div className="col-10 detail-table-cell">
          <Link
            to={`${cluster?.connection_url}`}
            target="_blank"
            className="data-table-link"
          >
            {cluster?.connection_url}
          </Link>
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Last updated</div>
        <div className="col-10 detail-table-cell">
          {formatDate(cluster?.last_updated || "")}
        </div>
      </div>
    </div>
  );
};

export default ClusterOverview;
