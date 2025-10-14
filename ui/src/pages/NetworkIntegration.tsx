import { Container } from "react-bootstrap";
import { useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { fetchNetworkIntegrations } from "api/network_integration";
import ExtendedDataTable from "components/ExtendedDataTable";
import InventorySearchBox from "components/InventorySearchBox";
import { formatDate } from "util/date";

const NetworkIntegration = () => {
  const [searchParams] = useSearchParams();
  const filter = searchParams.get("filter");

  const {
    data: network_integrations = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["network_integrations", filter],
    queryFn: () => fetchNetworkIntegrations(filter || ""),
    retry: false,
  });

  const headers = ["Name", "Cluster", "Last updated"];
  const rows = network_integrations.map((item) => {
    return [
      {
        content: item.name,
        sortKey: item.name,
      },
      {
        content: item.cluster,
        sortKey: item.cluster,
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

export default NetworkIntegration;
