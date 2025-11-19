import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import { fetchCluster } from "api/cluster";
import TabView from "components/TabView";
import ClusterArtifacts from "pages/ClusterArtifacts";
import ClusterOverview from "pages/ClusterOverview";

const ClusterDetail = () => {
  const navigate = useNavigate();
  const { name, activeTab } = useParams<{ name: string; activeTab: string }>();

  const {
    data: cluster = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["clusters", name],
    queryFn: () => fetchCluster(name || ""),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error || !cluster) {
    return <div>Error while loading cluster</div>;
  }

  const tabs = [
    {
      key: "overview",
      title: "Overview",
      content: <ClusterOverview />,
    },
    {
      key: "artifacts",
      title: "Artifacts",
      content: <ClusterArtifacts />,
    },
  ];

  return (
    <div className="d-flex flex-column">
      <div className="scroll-container flex-grow-1 p-3">
        <TabView
          defaultTab="overview"
          activeTab={activeTab}
          tabs={tabs}
          onSelect={(key) =>
            navigate(`/ui/provisioning/clusters/${name}/${key}`)
          }
        />
      </div>
    </div>
  );
};

export default ClusterDetail;
