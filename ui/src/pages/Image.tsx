import { useQuery } from "@tanstack/react-query";
import { fetchImages } from "api/image";
import DataTable from "components/DataTable";
import ObjectIncusLink from "components/ObjectIncusLink";
import ProjectIncusLink from "components/ProjectIncusLink";
import { formatDate } from "util/date";

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

  const headers = ["Name", "Cluster", "Project", "Last updated"];
  const rows = images.map((item) => {
    return [
      {
        content: (
          <ObjectIncusLink
            cluster={item.cluster}
            objectName={item.name}
            incusPath={`/ui/project/${item.project_name}/images`}
          />
        ),
        sortKey: item.name,
      },
      {
        content: item.cluster,
        sortKey: item.cluster,
      },
      {
        content: (
          <ProjectIncusLink
            cluster={item.cluster}
            project={item.project_name}
          />
        ),
        sortKey: item.project_name,
      },
      {
        content: formatDate(item.last_updated),
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
