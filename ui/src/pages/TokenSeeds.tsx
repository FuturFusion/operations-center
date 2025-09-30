import Button from "react-bootstrap/Button";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "react-router";
import { fetchTokenSeeds } from "api/token";
import DataTable from "components/DataTable.tsx";
import TokenSeedActions from "components/TokenSeedActions";
import { formatDate } from "util/date";

const TokenSeeds = () => {
  const navigate = useNavigate();
  const { uuid } = useParams();

  const {
    data: seeds = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["tokens", uuid, "seeds"],
    queryFn: () => fetchTokenSeeds(uuid || ""),
  });

  if (isLoading) {
    return <div>Loading seeds...</div>;
  }

  if (error) {
    return <div>Error while loading seeds: {error.message}</div>;
  }

  const headers = ["Name", "Public", "Last updated", ""];

  const rows = seeds.map((item) => {
    return [
      {
        content: (
          <Link
            to={`/ui/provisioning/tokens/${uuid}/seeds/${item.name}`}
            className="data-table-link"
          >
            {item.name}
          </Link>
        ),
        sortKey: item.name,
      },
      {
        content: item.public ? "Yes" : "No",
        sortKey: item.public ? "Yes" : "No",
      },
      {
        content: formatDate(item.last_updated || ""),
        sortKey: item.last_updated,
      },
      {
        content: <TokenSeedActions seed={item} />,
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
                onClick={() =>
                  navigate(`/ui/provisioning/tokens/${uuid}/seeds/create`)
                }
              >
                Create seed
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

export default TokenSeeds;
