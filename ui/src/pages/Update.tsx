import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import { fetchUpdates } from "api/update";
import DataTable from "components/DataTable";
import { formatDate } from "util/date";

const Update = () => {
  const {
    data: updates = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["updates"],
    queryFn: fetchUpdates,
  });

  if (isLoading) {
    return <div>Loading updates...</div>;
  }

  if (error) {
    return <div>Error while loading updates: {error.message}</div>;
  }

  const headers = ["Version", "Published at", "Severity", "Origin", "Channel"];
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
        <div className="scroll-container flex-grow-1">
          <DataTable headers={headers} rows={rows} />
        </div>
      </div>
    </>
  );
};

export default Update;
