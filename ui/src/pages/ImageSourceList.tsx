import Button from "react-bootstrap/Button";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router";
import { fetchImageSources } from "api/image_incus_source";
import Breadcrumbs from "components/Breadcrumbs";
import DataTable from "components/DataTable";
import ImageSourceActions from "components/ImageSourceActions";
import { formatDate } from "util/date";

const ImageSourceList = () => {
  const navigate = useNavigate();

  const {
    data: sources = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["image-sources"],
    queryFn: fetchImageSources,
  });

  if (isLoading) {
    return <div>Loading image sources...</div>;
  }

  if (error) {
    return <div>Error while loading image sources: {error.message}</div>;
  }

  const headers = [
    "Name",
    "URL",
    "Filter expression",
    "Last updated",
    "Actions",
  ];
  const rows = sources.map((item) => {
    return {
      cols: [
        {
          content: (
            <Link
              to={`/ui/images/incus/${item.name}`}
              className="data-table-link"
            >
              {item.name}
            </Link>
          ),
          sortKey: item.name,
        },
        {
          content: item.url,
          sortKey: item.url,
        },
        {
          content: item.filter_expression,
          sortKey: item.filter_expression,
        },
        {
          content: formatDate(item.last_updated),
          sortKey: item.last_updated,
        },
        {
          content: <ImageSourceActions source={item} />,
        },
      ],
    };
  });

  return (
    <div className="d-flex flex-column">
      <Breadcrumbs />
      <div className="scroll-container flex-grow-1 p-3">
        <div className="mx-2 mx-md-4">
          <div className="row">
            <div className="col-12">
              <Button
                variant="success"
                className="float-end"
                onClick={() => navigate("/ui/images/incus/create")}
              >
                Create source
              </Button>
            </div>
          </div>
        </div>
        <DataTable headers={headers} rows={rows} />
      </div>
    </div>
  );
};

export default ImageSourceList;
