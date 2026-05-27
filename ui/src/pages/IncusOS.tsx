import type { FC } from "react";
import { useNavigate, useParams } from "react-router";
import Breadcrumbs from "components/Breadcrumbs";
import TabView from "components/TabView";
import OSOverview from "./OSOverview";
import OSLogs from "./OSLogs";
import OSNetwork from "./OSNetwork";
import OSStorage from "./OSStorage";
import OSServices from "./OSServices";
import OSSecurity from "./OSSecurity";

const tabs = [
  {
    key: "overview",
    title: "Overview",
    content: <OSOverview />,
  },
  {
    key: "logs",
    title: "Logs",
    content: <OSLogs />,
  },
  {
    key: "network",
    title: "Network",
    content: <OSNetwork />,
  },
  {
    key: "storage",
    title: "Storage",
    content: <OSStorage />,
  },
  {
    key: "security",
    title: "Security",
    content: <OSSecurity />,
  },
  {
    key: "services",
    title: "Services",
    content: <OSServices />,
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
