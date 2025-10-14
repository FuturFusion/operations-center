import { Container } from "react-bootstrap";
import { useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { fetchImages } from "api/image";
import ExtendedDataTable from "components/ExtendedDataTable";
import InventorySearchBox from "components/InventorySearchBox";
import ObjectIncusLink from "components/ObjectIncusLink";
import ProjectIncusLink from "components/ProjectIncusLink";
import { formatDate } from "util/date";

const Image = () => {
  const [searchParams] = useSearchParams();
  const filter = searchParams.get("filter");

  const {
    data: images = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["images", filter],
    queryFn: () => fetchImages(filter || ""),
    retry: false,
  });

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
      <Container className="d-flex justify-content-center">
        <InventorySearchBox />
      </Container>
      <div className="d-flex flex-column">
        <div className="scroll-container flex-grow-1">
          <ExtendedDataTable
            headers={headers}
            rows={rows}
            isLoading={isLoading}
            error={error}
          />
        </div>
      </div>
    </>
  );
};

export default Image;
