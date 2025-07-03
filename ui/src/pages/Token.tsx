import Button from "react-bootstrap/Button";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router";
import { fetchTokens } from "api/token";
import DataTable from "components/DataTable";
import { formatDate } from "util/date";

const Token = () => {
  const navigate = useNavigate();

  const {
    data: tokens = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["tokens"],
    queryFn: fetchTokens,
  });

  if (isLoading) {
    return <div>Loading tokens...</div>;
  }

  if (error) {
    return <div>Error while loading tokens: {error.message}</div>;
  }

  const headers = ["UUID", "Description", "Expiry", "Remaining uses"];
  const rows = tokens.map((item) => {
    return [
      {
        content: (
          <Link
            to={`/ui/provisioning/tokens/${item.uuid}`}
            className="data-table-link"
          >
            {item.uuid}
          </Link>
        ),
        sortKey: item.uuid,
      },
      {
        content: item.description,
        sortKey: item.description,
      },
      {
        content: formatDate(item.expire_at || ""),
        sortKey: item.expire_at,
      },
      {
        content: item.uses_remaining,
        sortKey: item.uses_remaining,
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
                onClick={() => navigate("/ui/provisioning/tokens/create")}
              >
                Issue token
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

export default Token;
