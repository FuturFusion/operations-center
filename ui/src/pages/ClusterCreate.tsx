import { useNavigate, useParams } from "react-router";
import TabView from "components/TabView";
import ClusterCreateManual from "pages/ClusterCreateManual";

const ClusterCreate = () => {
  const navigate = useNavigate();
  const { activeTab } = useParams<{ activeTab: string }>();

  const tabs = [
    {
      key: "manual",
      title: "Manually",
      content: <ClusterCreateManual />,
    },
  ];

  return (
    <div className="d-flex flex-column">
      <div className="scroll-container flex-grow-1 p-3">
        <TabView
          defaultTab="manual"
          activeTab={activeTab}
          tabs={tabs}
          onSelect={(key) =>
            navigate(`/ui/provisioning/clusters/create/${key}`)
          }
        />
      </div>
    </div>
  );
};

export default ClusterCreate;
