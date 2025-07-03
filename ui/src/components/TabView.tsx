import { FC, ReactNode } from "react";
import Tab from "react-bootstrap/Tab";
import Tabs from "react-bootstrap/Tabs";

interface Tab {
  key: string;
  title: string;
  content: ReactNode;
}

interface TabViewProps {
  tabs: Tab[];
  defaultTab: string;
  activeTab: string | undefined;
  onSelect: (key: string | null) => void;
}

const TabView: FC<TabViewProps> = ({
  tabs,
  defaultTab,
  activeTab,
  onSelect,
}) => {
  const activeKey = activeTab && activeTab != "" ? activeTab : defaultTab;

  const getTabs = () => {
    return tabs.map((item) => {
      return (
        <Tab eventKey={item.key} title={item.title}>
          {(!activeTab || activeTab == item.key) && item.content}
        </Tab>
      );
    });
  };

  return (
    <Tabs
      defaultActiveKey={activeKey}
      id="uncontrolled-tab-example"
      className="mb-3"
      onSelect={(key) => onSelect(key)}
    >
      {getTabs()}
    </Tabs>
  );
};

export default TabView;
