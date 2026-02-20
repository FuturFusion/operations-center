import { useNavigate, useParams } from "react-router";
import TabView from "components/TabView";
import ClusterList from "pages/ClusterList";
import ClusterTemplate from "pages/ClusterTemplate";

const Cluster = () => {
  const navigate = useNavigate();
  const { activeTab } = useParams<{ activeTab: string }>();

  const tabs = [
    {
      key: "clusters",
      title: "Clusters",
      content: <ClusterList />,
    },
    {
      key: "templates",
      title: "Templates",
      content: <ClusterTemplate />,
    },
  ];

  return (
    <div className="d-flex flex-column">
      <div className="scroll-container flex-grow-1 p-3">
        <TabView
          defaultTab="clusters"
          activeTab={activeTab}
          tabs={tabs}
          onSelect={(key) => navigate(`/ui/provisioning/clusters-view/${key}`)}
        />
      </div>
    </div>
  );
};

export default Cluster;
