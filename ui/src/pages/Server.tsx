import { useNavigate, useParams } from "react-router";
import TabView from "components/TabView";
import ServerList from "pages/ServerList";
import Token from "pages/Token";

const Server = () => {
  const navigate = useNavigate();
  const { activeTab } = useParams<{ activeTab: string }>();

  const tabs = [
    {
      key: "servers",
      title: "Servers",
      content: <ServerList />,
    },
    {
      key: "tokens",
      title: "Tokens",
      content: <Token />,
    },
  ];

  return (
    <div className="d-flex flex-column">
      <div className="scroll-container flex-grow-1 p-3">
        <TabView
          defaultTab="servers"
          activeTab={activeTab}
          tabs={tabs}
          onSelect={(key) => navigate(`/ui/provisioning/servers-view/${key}`)}
        />
      </div>
    </div>
  );
};

export default Server;
