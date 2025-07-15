import { useQuery } from "@tanstack/react-query";
import { fetchNetworkACLs } from "api/network_acl";
import DataTable from "components/DataTable";

const NetworkACL = () => {
  const {
    data: acls = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["network_acls"],
    queryFn: fetchNetworkACLs,
  });

  if (isLoading) {
    return <div>Loading network ACLs...</div>;
  }

  if (error) {
    return <div>Error while loading network ACLs: {error.message}</div>;
  }

  const headers = ["Name", "Cluster", "Project name", "Last updated"];
  const rows = acls.map((item) => {
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
        content: item.project_name,
        sortKey: item.project_name,
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

export default NetworkACL;
