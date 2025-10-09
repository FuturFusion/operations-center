import Button from "react-bootstrap/Button";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import { fetchUpdates, refreshUpdates } from "api/update";
import DataTable from "components/DataTable";
import { useNotification } from "context/notificationContext";
import { formatDate } from "util/date";

const Update = () => {
  const { notify } = useNotification();

  const {
    data: updates = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["updates"],
    queryFn: fetchUpdates,
  });

  const handleRefresh = () => {
    refreshUpdates()
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Updates refresh triggered successfully`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during refreshing updates: ${e}`);
      });
  };

  if (isLoading) {
    return <div>Loading updates...</div>;
  }

  if (error) {
    return <div>Error while loading updates: {error.message}</div>;
  }

  const headers = [
    "Version",
    "Published at",
    "Status",
    "Severity",
    "Origin",
    "Channel",
  ];
  const rows = updates.map((item) => {
    return [
      {
        content: (
          <Link
            to={`/ui/provisioning/updates/${item.uuid}`}
            className="data-table-link"
          >
            {item.version}
          </Link>
        ),
        sortKey: item.version,
      },
      {
        content: formatDate(item.published_at),
        sortKey: item.published_at,
      },
      {
        content: item.update_status,
        sortKey: item.update_status,
      },
      {
        content: item.severity,
        sortKey: item.severity,
      },
      {
        content: item.origin,
        sortKey: item.origin,
      },
      {
        content: item.channel,
        sortKey: item.channel,
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
                onClick={handleRefresh}
              >
                Refresh
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

export default Update;
