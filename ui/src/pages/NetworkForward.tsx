import { useQuery } from "@tanstack/react-query";
import { fetchNetworkForwards } from "api/network_forward";
import DataTable from "components/DataTable";
import ObjectIncusLink from "components/ObjectIncusLink";
import { formatDate } from "util/date";

const NetworkForward = () => {
  const {
    data: network_forwards = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["network_forwards"],
    queryFn: fetchNetworkForwards,
  });

  if (isLoading) {
    return <div>Loading network forwards...</div>;
  }

  if (error) {
    return <div>Error while loading network forwards: {error.message}</div>;
  }

  const headers = ["Name", "Cluster", "Network name", "Last updated"];
  const rows = network_forwards.map((item) => {
    return [
      {
        content: (
          <ObjectIncusLink
            cluster={item.cluster}
            objectName={item.name}
            incusPath={`/ui/project/default/network/${item.parent_name}/forwards`}
          />
        ),
        sortKey: item.name,
      },
      {
        content: item.cluster,
        sortKey: item.cluster,
      },
      {
        content: item.parent_name,
        sortKey: item.parent_name,
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

export default NetworkForward;
