import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router";
import { fetchIncusImage } from "api/image_incus";
import { formatDate } from "util/date";

const IncusImageOverview = () => {
  const { name } = useParams();

  const {
    data: image = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["incus-images", name],
    queryFn: () => fetchIncusImage(name || ""),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading image</div>;
  }

  return (
    <div className="container">
      <div className="row">
        <div className="col-2 detail-table-header">Name</div>
        <div className="col-10 detail-table-cell">{image?.name}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Aliases</div>
        <div className="col-10 detail-table-cell">
          {(image?.aliases ?? []).join(", ")}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Operating system</div>
        <div className="col-10 detail-table-cell">{image?.os}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Release</div>
        <div className="col-10 detail-table-cell">{image?.release}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Architecture</div>
        <div className="col-10 detail-table-cell">{image?.arch}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Variant</div>
        <div className="col-10 detail-table-cell">{image?.variant}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Description</div>
        <div className="col-10 detail-table-cell">{image?.description}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Last updated</div>
        <div className="col-10 detail-table-cell">
          {formatDate(image?.last_updated || "")}
        </div>
      </div>
    </div>
  );
};

export default IncusImageOverview;
