import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router";
import { fetchChannel } from "api/channel";
import { useServers } from "context/useServers";
import { fetchUpdates } from "api/update";
import { formatDate } from "util/date";

const ChannelOverview = () => {
  const { name } = useParams();
  const { data: servers } = useServers("");

  const { data: updates = [] } = useQuery({
    queryKey: ["updates"],
    queryFn: () => fetchUpdates(),
  });

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
      <div className="row">
        <div className="col-2 detail-table-header">Servers</div>
        <div className="col-10 detail-table-cell">
          {servers
            ?.filter((s) => s.channel === name)
            .map((server) => (
              <div>
                <Link
                  to={`/ui/provisioning/servers/${server.name}`}
                  className="data-table-link"
                >
                  {server.name}
                </Link>
              </div>
            ))}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Updates</div>
        <div className="col-10 detail-table-cell">
          {updates
            ?.filter((u) => u.channels.includes(name ?? ""))
            .map((u) => (
              <div>
                <Link
                  to={`/ui/provisioning/updates/${u.uuid}`}
                  className="data-table-link"
                >
                  {u.uuid}
                </Link>
              </div>
            ))}
        </div>
      </div>
    </div>
  );
};

export default ChannelOverview;
