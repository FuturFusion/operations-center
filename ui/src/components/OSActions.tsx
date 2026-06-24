import type { FC } from "react";
import {
  MdOutlineBackup,
  MdOutlinePauseCircleOutline,
  MdOutlineRestore,
  MdOutlineSettingsBackupRestore,
} from "react-icons/md";
import { useQuery } from "@tanstack/react-query";
import {
  fetchSystemUpdate,
  runOSAction,
  runOSActionDownload,
  runOSActionUpload,
} from "api/os";
import ActionGroup from "components/ActionGroup";
import type { OSActionInput } from "components/OSAction";
import OSAction from "components/OSAction";
import RebootOSBtn from "components/RebootOSBtn";
import ShutdownOSBtn from "components/ShutdownOSBtn";

const OSActions: FC = () => {
  const { data: systemUpdate } = useQuery({
    queryKey: ["os-update"],
    queryFn: async () => fetchSystemUpdate(),
  });

  const pendingReboot = systemUpdate?.state?.needs_reboot ?? false;

  const powerItems = [
    <RebootOSBtn key="reboot" pending={pendingReboot} />,
    <OSAction
      key="suspend"
      label="Suspend"
      mode="confirm"
      icon={<MdOutlinePauseCircleOutline size={25} />}
      confirmMessage="Are you sure you want to suspend the system?"
      run={() => runOSAction("system", "suspend")}
      successMessage="System suspended"
    />,
    <ShutdownOSBtn key="shutdown" />,
  ];

  const maintenanceItems = [
    <OSAction
      key="backup"
      label="Backup"
      mode="download"
      icon={<MdOutlineBackup size={25} />}
      filename="incus-os-backup"
      confirmMessage="Download a backup of the system?"
      run={() => runOSActionDownload("system", "backup", {})}
      successMessage="System backup downloaded"
    />,
    <OSAction
      key="restore"
      label="Restore"
      mode="upload"
      icon={<MdOutlineRestore size={25} />}
      run={(input: OSActionInput) =>
        runOSActionUpload("system", "restore", input as File)
      }
      successMessage="System restore triggered"
    />,
    <OSAction
      key="factory-reset"
      label="Factory reset"
      mode="confirm"
      icon={<MdOutlineSettingsBackupRestore size={25} />}
      confirmMessage="This will factory-reset the system. Are you sure?"
      run={() => runOSAction("system", "factory-reset", {})}
      successMessage="Factory reset triggered"
    />,
  ];

  return <ActionGroup groups={[powerItems, maintenanceItems]} />;
};

export default OSActions;
