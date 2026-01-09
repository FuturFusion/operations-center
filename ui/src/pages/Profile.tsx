import { Container } from "react-bootstrap";
import { useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { fetchProfiles } from "api/profile";
import ClusterLink from "components/ClusterLink";
import ExtendedDataTable from "components/ExtendedDataTable";
import InventorySearchBox from "components/InventorySearchBox";
import ObjectIncusLink from "components/ObjectIncusLink";
import ProjectIncusLink from "components/ProjectIncusLink";
import type { Profile } from "types/profile";
import { formatDate } from "util/date";

const Profile = () => {
  const [searchParams] = useSearchParams();
  const filter = searchParams.get("filter");

  const sortData = (a: Profile, b: Profile) => {
    return (
      a.cluster.localeCompare(b.cluster) ||
      a.project_name.localeCompare(b.project_name) ||
      a.name.localeCompare(b.name)
    );
  };

  const {
    data: profiles = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["profiles", filter],
    queryFn: () => fetchProfiles(filter || ""),
    select: (items) => [...items].sort(sortData),
    retry: false,
  });

  const headers = ["Name", "Project", "Cluster", "Last updated"];
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
        content: (
          <ProjectIncusLink
            cluster={item.cluster}
            project={item.project_name}
          />
        ),
        sortKey: item.project_name,
      },
      {
        content: <ClusterLink cluster={item.cluster} />,
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

export default Profile;
