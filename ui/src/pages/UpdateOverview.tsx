import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router";
import { fetchUpdate } from "api/update";
import { formatDate } from "util/date";

const UpdateOverview = () => {
  const { uuid } = useParams();

  const {
    data: update = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["updates", uuid],
    queryFn: () => fetchUpdate(uuid || ""),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading update</div>;
  }

  return (
    <div className="container">
      <div className="row">
        <div className="col-2 detail-table-header">UUID</div>
        <div className="col-10 detail-table-cell">{update?.uuid}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Version</div>
        <div className="col-10 detail-table-cell">{update?.version}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Published at</div>
        <div className="col-10 detail-table-cell">
          {formatDate(update?.published_at || "")}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Status</div>
        <div className="col-10 detail-table-cell">{update?.update_status}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Severity</div>
        <div className="col-10 detail-table-cell">{update?.severity}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Origin</div>
        <div className="col-10 detail-table-cell">{update?.origin}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Upstream Channels</div>
        <div className="col-10 detail-table-cell">
          {(update?.upstream_channels ?? []).join(", ")}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Changelog</div>
        <div className="col-10 detail-table-cell">{update?.changelog}</div>
      </div>
    </div>
  );
};

export default UpdateOverview;
