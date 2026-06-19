import { useNavigate, useParams } from "react-router";
import Breadcrumbs from "components/Breadcrumbs";
import TabView from "components/TabView";
import IncusImageList from "pages/IncusImageList";
import ImageSourceList from "pages/ImageSourceList";

const ImageServer = () => {
  const navigate = useNavigate();
  const { activeTab } = useParams<{ activeTab: string }>();

  const tabs = [
    {
      key: "images",
      title: "Images",
      content: <IncusImageList />,
    },
    {
      key: "sources",
      title: "Sources",
      content: <ImageSourceList />,
    },
  ];

  return (
    <div className="d-flex flex-column">
      <Breadcrumbs />
      <div className="scroll-container flex-grow-1 p-3">
        <TabView
          defaultTab="images"
          activeTab={activeTab}
          tabs={tabs}
          onSelect={(key) => navigate(`/ui/images-view/${key}`)}
        />
      </div>
    </div>
  );
};

export default ImageServer;
