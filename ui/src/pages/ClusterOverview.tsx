import { Badge } from "react-bootstrap";
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router";
import { fetchCluster } from "api/cluster";
import { formatDate } from "util/date";
import { RestoreModeValues } from "util/cluster";

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
        <div className="col-2 detail-table-header">Description</div>
        <div className="col-10 detail-table-cell">{cluster?.description}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Properties</div>
        <div className="col-10 detail-table-cell">
          {cluster?.properties &&
            Object.entries(cluster?.properties).map(([key, value]) => [
              <Badge bg="primary">
                {key}:{value}
              </Badge>,
              <span> </span>,
            ])}
        </div>
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
        <div className="col-2 detail-table-header">Channel</div>
        <div className="col-10 detail-table-cell">{cluster?.channel}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Restore mode</div>
        <div className="col-10 detail-table-cell">
          {
            RestoreModeValues[
              (cluster?.config.rolling_restart.restore_mode ??
                "") as keyof typeof RestoreModeValues
            ]
          }
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Post restore delay</div>
        <div className="col-10 detail-table-cell">
          {(cluster?.config.rolling_restart.post_restore_delay || 0) /
            1000 /
            1000 /
            60}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Certificate</div>
        <div className="col-10 detail-table-cell">
          <pre>{cluster?.certificate}</pre>
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Fingerprint</div>
        <div className="col-10 detail-table-cell">{cluster?.fingerprint}</div>
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
