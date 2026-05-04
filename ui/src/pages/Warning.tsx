import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import { fetchWarnings } from "api/warnings";
import Breadcrumbs from "components/Breadcrumbs";
import DataTable from "components/DataTable";
import { formatDate } from "util/date";

const Warning = () => {
  const {
    data: warnings = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["warnings"],
    queryFn: fetchWarnings,
  });

  const headers = ["Type", "Status", "Count", "First seen", "Last seen"];
  const rows = warnings.map((item) => {
    const messages = item.messages.map((message) => {
      return (
        <>
          <div
            style={{
              display: "flex",
              justifyContent: "space-between",
              marginBottom: "4px",
            }}
          >
            - {message}
          </div>
        </>
      );
    });

    return {
      cols: [
        {
          content: (
            <Link
              to={`/ui/provisioning/warnings/${item.uuid}`}
              className="data-table-link"
            >
              {item.type}
            </Link>
          ),
          sortKey: item.type,
        },
        {
          content: item.status,
          sortKey: item.status,
        },
        {
          content: item.count,
          sortKey: item.count,
        },
        {
          content: formatDate(item.first_occurrence),
          sortKey: formatDate(item.first_occurrence),
        },
        {
          content: formatDate(item.last_occurrence),
          sortKey: formatDate(item.last_occurrence),
        },
      ],
      additional_data: item.messages.length > 0 && (
        <>
          <b style={{ fontSize: "14px" }}>Messages</b> {messages}
        </>
      ),
    };
  });

  if (isLoading) {
    return <div>Loading warnings...</div>;
  }

  if (error) {
    return <div>Error while loading warnings</div>;
  }

  return (
    <div className="d-flex flex-column">
      <Breadcrumbs />
      <div className="scroll-container flex-grow-1 p-3">
        <DataTable headers={headers} rows={rows} />
      </div>
    </div>
  );
};

export default Warning;
