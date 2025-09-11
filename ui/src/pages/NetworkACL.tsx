import { useQuery } from "@tanstack/react-query";
import { fetchNetworkACLs } from "api/network_acl";
import DataTable from "components/DataTable";
import ObjectIncusLink from "components/ObjectIncusLink";
import ProjectIncusLink from "components/ProjectIncusLink";
import { formatDate } from "util/date";

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

  const headers = ["Name", "Cluster", "Project", "Last updated"];
  const rows = acls.map((item) => {
    return [
      {
        content: (
          <ObjectIncusLink
            cluster={item.cluster}
            objectName={item.name}
            incusPath={`/ui/project/${item.project_name}/network-acl/${item.name}`}
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

export default NetworkACL;
