import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router";
import { fetchImageSource } from "api/image_incus_source";
import { formatDate } from "util/date";

const ImageSourceOverview = () => {
  const { name } = useParams();

  const {
    data: source = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["image-sources", name],
    queryFn: () => fetchImageSource(name || ""),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading image source</div>;
  }

  return (
    <div className="container">
      <div className="row">
        <div className="col-2 detail-table-header">Name</div>
        <div className="col-10 detail-table-cell">{source?.name}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">URL</div>
        <div className="col-10 detail-table-cell">{source?.url}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Filter expression</div>
        <div className="col-10 detail-table-cell">
          {source?.filter_expression}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Last updated</div>
        <div className="col-10 detail-table-cell">
          {formatDate(source?.last_updated || "")}
        </div>
      </div>
    </div>
  );
};

export default ImageSourceOverview;
