import { useNavigate, useParams } from "react-router";
import TabView from "components/TabView";
import SystemCertConfiguration from "components/SystemCertConfiguration";
import SystemNetworkConfiguration from "components/SystemNetworkConfiguration";
import SystemSecurityConfiguration from "components/SystemSecurityConfiguration";
import SystemSettingsConfiguration from "components/SystemSettingsConfiguration";
import SystemUpdatesConfiguration from "components/SystemUpdatesConfiguration";

const Settings = () => {
  const { activeTab } = useParams<{ activeTab: string }>();
  const navigate = useNavigate();

  const tabs = [
    {
      key: "network",
      title: "Network",
      content: <SystemNetworkConfiguration />,
    },
    {
      key: "security",
      title: "Security",
      content: <SystemSecurityConfiguration />,
    },
    {
      key: "settings",
      title: "Settings",
      content: <SystemSettingsConfiguration />,
    },
    {
      key: "certificate",
      title: "Certificate",
      content: <SystemCertConfiguration />,
    },
    {
      key: "updates",
      title: "Updates",
      content: <SystemUpdatesConfiguration />,
    },
  ];

  return (
    <div className="d-flex flex-column">
      <div className="scroll-container flex-grow-1 p-3">
        <TabView
          defaultTab="network"
          activeTab={activeTab}
          tabs={tabs}
          onSelect={(key) => navigate(`/ui/settings/${key}`)}
        />
      </div>
    </div>
  );
};

export default Settings;
