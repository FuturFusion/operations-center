import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import { fetchIncusImages } from "api/image_incus";
import DataTable from "components/DataTable";
import IncusImageActions from "components/IncusImageActions";
import UploadIncusImageBtn from "components/UploadIncusImageBtn";
import { formatDate } from "util/date";

const IncusImageList = () => {
  const {
    data: images = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["incus-images"],
    queryFn: fetchIncusImages,
  });

  if (isLoading) {
    return <div>Loading images...</div>;
  }

  if (error) {
    return <div>Error while loading images: {error.message}</div>;
  }

  const headers = [
    "Name",
    "Operating system",
    "Release",
    "Architecture",
    "Variant",
    "Description",
    "Last updated",
    "Actions",
  ];
  const rows = images.map((item) => {
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
          content: item.os,
          sortKey: item.os,
        },
        {
          content: item.release,
          sortKey: item.release,
        },
        {
          content: item.arch,
          sortKey: item.arch,
        },
        {
          content: item.variant,
          sortKey: item.variant,
        },
        {
          content: item.description,
          sortKey: item.description,
        },
        {
          content: formatDate(item.last_updated),
          sortKey: item.last_updated,
        },
        {
          content: <IncusImageActions image={item} />,
        },
      ],
    };
  });

  return (
    <>
      <div className="d-flex flex-column">
        <div className="mx-2 mx-md-4">
          <div className="row">
            <div className="col-12">
              <UploadIncusImageBtn />
            </div>
          </div>
        </div>
        <div className="flex-grow-1">
          <DataTable headers={headers} rows={rows} />
        </div>
      </div>
    </>
  );
};

export default IncusImageList;
