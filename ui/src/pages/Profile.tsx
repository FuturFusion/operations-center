import { useQuery } from "@tanstack/react-query";
import { fetchProfiles } from "api/profile";
import DataTable from "components/DataTable";
import ObjectIncusLink from "components/ObjectIncusLink";
import ProjectIncusLink from "components/ProjectIncusLink";
import { formatDate } from "util/date";

const Profile = () => {
  const {
    data: profiles = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["profiles"],
    queryFn: fetchProfiles,
  });

  if (isLoading) {
    return <div>Loading profiles...</div>;
  }

  if (error) {
    return <div>Error while loading profiles: {error.message}</div>;
  }

  const headers = ["Name", "Cluster", "Project", "Last updated"];
  const rows = profiles.map((item) => {
    return [
      {
        content: (
          <ObjectIncusLink
            cluster={item.cluster}
            objectName={item.name}
            incusPath={`/ui/project/${item.project_name}/profile/${item.name}`}
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

export default Profile;
