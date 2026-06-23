import type { FC } from "react";
import { useNavigate, useParams } from "react-router";
import Breadcrumbs from "components/Breadcrumbs";
import OSActions from "components/OSActions";
import TabView from "components/TabView";
import OSOverview from "./OSOverview";
import OSApplications from "./OSApplications";
import OSDebug from "./OSDebug";
import OSServices from "./OSServices";
import OSSystem from "./OSSystem";

const tabs = [
  {
    key: "overview",
    title: "Overview",
    content: <OSOverview />,
  },
  {
    key: "applications",
    title: "Applications",
    content: <OSApplications />,
  },
  {
    key: "debug",
    title: "Debug",
    content: <OSDebug />,
  },
  {
    key: "services",
    title: "Services",
    content: <OSServices />,
  },
  {
    key: "system",
    title: "System",
    content: <OSSystem />,
  },
];

const IncusOS: FC = () => {
  const navigate = useNavigate();
  const { activeTab } = useParams<{
    activeTab?: string;
  }>();

  return (
    <div className="d-flex flex-column">
      <Breadcrumbs />
      <div className="d-flex justify-content-end mt-2">
        <OSActions />
      </div>
      <div className="scroll-container flex-grow-1 p-3">
        <TabView
          defaultTab="overview"
          activeTab={activeTab}
          tabs={tabs}
          onSelect={(key) => navigate(`/ui/os/${key}`)}
        />
      </div>
    </div>
  );
};

export default IncusOS;
