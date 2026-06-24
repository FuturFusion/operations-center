import type { FC } from "react";
import { useNavigate, useParams } from "react-router";
import { runOSAction } from "api/os";
import OSAction from "components/OSAction";
import OSConfigSection from "components/OSConfigSection";
import TabView from "components/TabView";
import OSDebugLog from "./OSDebugLog";
import OSDebugProcesses from "./OSDebugProcesses";

const SecureBootActions: FC = () => (
  <OSAction
    label="Update keys"
    mode="confirm"
    confirmMessage="Update the secure boot keys?"
    run={() => runOSAction("debug/secureboot", "update")}
    successMessage="Secure boot keys update triggered"
  />
);

const subTabs = [
  {
    key: "log",
    title: "Log",
    content: <OSDebugLog />,
  },
  {
    key: "processes",
    title: "Processes",
    content: <OSDebugProcesses />,
  },
  {
    key: "secureboot",
    title: "Secure Boot",
    content: (
      <OSConfigSection
        endpoint="debug/secureboot/event-log"
        queryKey="os-secureboot-event-log"
        label="Secure boot event log"
        readOnly
        actions={<SecureBootActions />}
      />
    ),
  },
].sort((a, b) => a.title.localeCompare(b.title));

const OSDebug: FC = () => {
  const navigate = useNavigate();
  const { subTab } = useParams<{ subTab?: string }>();

  return (
    <TabView
      defaultTab={subTabs[0].key}
      activeTab={subTab}
      tabs={subTabs}
      onSelect={(key) => navigate(`/ui/os/debug/${key}`)}
    />
  );
};

export default OSDebug;
