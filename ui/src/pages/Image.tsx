import { Container } from "react-bootstrap";
import { useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { fetchImages } from "api/image";
import ExtendedDataTable from "components/ExtendedDataTable";
import InventorySearchBox from "components/InventorySearchBox";
import ObjectIncusLink from "components/ObjectIncusLink";

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

  const headers = ["Fingerprint", "Image description", "Image type"];
  const rows = images.map((item) => {
    return [
      {
        content: (
          <ObjectIncusLink
            cluster={item.cluster}
            objectName={item.object.fingerprint.slice(0, 12)}
            incusPath={`/ui/project/${item.project_name}/images`}
          />
        ),
        sortKey: item.object.fingerprint,
      },
      {
        content: item.object.properties?.description,
        sortKey: item.object.properties?.description,
      },
      {
        content: item.object.type,
        sortKey: item.object.type,
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
