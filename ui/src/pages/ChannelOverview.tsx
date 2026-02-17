import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router";
import { fetchChannel } from "api/channel";
import { formatDate } from "util/date";

const ChannelOverview = () => {
  const { name } = useParams();

  const {
    data: channel = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["channels", name],
    queryFn: () => fetchChannel(name || ""),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading channel</div>;
  }

  return (
    <div className="container">
      <div className="row">
        <div className="col-2 detail-table-header">Name</div>
        <div className="col-10 detail-table-cell">{channel?.name}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">description</div>
        <div className="col-10 detail-table-cell">{channel?.description}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Last updated</div>
        <div className="col-10 detail-table-cell">
          {formatDate(channel?.last_updated || "")}
        </div>
      </div>
    </div>
  );
};

export default ChannelOverview;
