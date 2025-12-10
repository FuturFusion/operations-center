import Button from "react-bootstrap/Button";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router";
import { fetchClusterTemplates } from "api/cluster_template";
import DataTable from "components/DataTable";
import type { ClusterTemplate } from "types/cluster_template";
import { formatDate } from "util/date";

const ClusterTemplate = () => {
  const navigate = useNavigate();

  const sortData = (a: ClusterTemplate, b: ClusterTemplate) => {
    return a.name.localeCompare(b.name);
  };

  const {
    data: templates = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["cluster-templates"],
    queryFn: fetchClusterTemplates,
    select: (items) => [...items].sort(sortData),
  });

  if (isLoading) {
    return <div>Loading cluster templates...</div>;
  }

  if (error) {
    return <div>Error while loading cluster templates: {error.message}</div>;
  }

  const headers = ["Name", "Description", "Last updated"];
  const rows = templates.map((item) => {
    return [
      {
        content: (
          <Link
            to={`/ui/provisioning/cluster-templates/${item.name}`}
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
        content: formatDate(item.last_updated || ""),
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
                onClick={() =>
                  navigate("/ui/provisioning/cluster-templates/create")
                }
              >
                Create template
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

export default ClusterTemplate;
