import { useNavigate, useParams } from "react-router";
import TabView from "components/TabView";
import UpdateList from "pages/UpdateList";
import Channel from "pages/Channel";

const Update = () => {
  const navigate = useNavigate();
  const { activeTab } = useParams<{ activeTab: string }>();

  const tabs = [
    {
      key: "images",
      title: "Images",
      content: <UpdateList />,
    },
    {
      key: "channels",
      title: "Channels",
      content: <Channel />,
    },
  ];

  return (
    <div className="d-flex flex-column">
      <div className="scroll-container flex-grow-1 p-3">
        <TabView
          defaultTab="images"
          activeTab={activeTab}
          tabs={tabs}
          onSelect={(key) => navigate(`/ui/provisioning/updates-view/${key}`)}
        />
      </div>
    </div>
  );
};

export default Update;
