import Button from "react-bootstrap/Button";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router";
import { fetchChannels } from "api/channel";
import DataTable from "components/DataTable";
import { formatDate } from "util/date";

const Channel = () => {
  const navigate = useNavigate();

  const {
    data: channels = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["channels"],
    queryFn: fetchChannels,
  });

  if (isLoading) {
    return <div>Loading updates...</div>;
  }

  if (error) {
    return <div>Error while loading channels: {error.message}</div>;
  }

  const headers = ["Name", "Description", "Last updated"];
  const rows = channels.map((item) => {
    return [
      {
        content: (
          <Link
            to={`/ui/provisioning/channels/${item.name}`}
            className="data-table-link"
          >
            {item.name}
          </Link>
        ),
        sortKey: item.name,
      },
      {
        content: item.description,
        sortKey: item.description,
      },
      {
        content: formatDate(item.last_updated),
        sortKey: item.last_updated,
      },
    ];
  });

  return (
    <>
      <div className="d-flex flex-column">
        <div className="mx-2 mx-md-4">
          <div className="row">
            <div className="col-12">
              <Button
                variant="success"
                className="float-end"
                onClick={() => navigate("/ui/provisioning/channels/create")}
              >
                Create channel
              </Button>
            </div>
          </div>
        </div>
        <div className="scroll-container flex-grow-1">
          <DataTable headers={headers} rows={rows} />
        </div>
      </div>
    </>
  );
};

export default Channel;
