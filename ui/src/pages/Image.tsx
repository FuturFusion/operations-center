import { useQuery } from "@tanstack/react-query";
import { fetchImages } from "api/images";
import DataTable from "components/DataTable";

const Image = () => {
  const {
    data: images = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["images"],
    queryFn: fetchImages,
  });

  if (isLoading) {
    return <div>Loading images...</div>;
  }

  if (error) {
    return <div>Error while loading images: {error.message}</div>;
  }

  const headers = ["UUID", "Cluster", "Project name", "Name", "Last updated"];
  const rows = images.map((item) => {
    return [
      {
        content: item.uuid,
        sortKey: item.uuid,
      },
      {
        content: item.cluster,
        sortKey: item.cluster,
      },
      {
        content: item.project_name,
        sortKey: item.project_name,
      },
      {
        content: item.name,
        sortKey: item.name,
      },
      {
        content: item.last_updated,
        sortKey: item.last_updated,
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

export default Image;
