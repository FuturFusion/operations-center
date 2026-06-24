import type { FC } from "react";
import { useNavigate, useParams } from "react-router";
import { runOSAction } from "api/os";
import OSAction from "components/OSAction";
import type { OSActionInput, OSActionValues } from "components/OSAction";
import OSConfigSection from "components/OSConfigSection";
import TabView from "components/TabView";

const str = (v: OSActionValues, k: string) => String(v[k] ?? "");
const num = (v: OSActionValues, k: string) => Number(v[k] ?? 0);
const bool = (v: OSActionValues, k: string) => Boolean(v[k]);

const storageRun =
  (action: string, body: (v: OSActionValues) => object) =>
  (input: OSActionInput) =>
    runOSAction("system/storage", action, body(input as OSActionValues));

const createVolume = storageRun("create-volume", (v) => ({
  pool: str(v, "pool"),
  name: str(v, "name"),
  quota: num(v, "quota"),
  use: str(v, "use"),
}));
const deleteVolume = storageRun("delete-volume", (v) => ({
  pool: str(v, "pool"),
  name: str(v, "name"),
  force: bool(v, "force"),
}));
const deletePool = storageRun("delete-pool", (v) => ({ name: str(v, "name") }));
const scrubPool = storageRun("scrub-pool", (v) => ({ name: str(v, "name") }));
const encryptDrive = storageRun("encrypt-drive", (v) => ({
  id: str(v, "id"),
  secure_wipe: bool(v, "secure_wipe"),
}));
const wipeDrive = storageRun("wipe-drive", (v) => ({
  id: str(v, "id"),
  secure_wipe: bool(v, "secure_wipe"),
}));
const importEncryptedDrive = storageRun("import-encrypted-drive", (v) => ({
  id: str(v, "id"),
  key: str(v, "key"),
}));
const importPool = storageRun("import-pool", (v) => ({
  name: str(v, "name"),
  type: "zfs",
  encryption_key: str(v, "encryption_key"),
}));

const NetworkActions: FC = () => (
  <>
    <OSAction
      label="Confirm configuration"
      mode="confirm"
      confirmMessage="Confirm the new network configuration?"
      run={() => runOSAction("system/network", "confirm")}
      successMessage="Network configuration confirmed"
    />
    <OSAction
      label="Flush DNS cache"
      mode="confirm"
      confirmMessage="Flush the DNS cache?"
      run={() => runOSAction("system/network", "flush-dns")}
      successMessage="DNS cache flushed"
    />
  </>
);

const SecurityActions: FC = () => (
  <OSAction
    label="Rebind TPM"
    mode="confirm"
    confirmMessage="Rebind the TPM and reboot the system?"
    run={() => runOSAction("system/security", "tpm-rebind")}
    successMessage="TPM rebind triggered"
  />
);

const StorageActions: FC = () => (
  <>
    <OSAction
      label="Create volume"
      mode="fields"
      submitLabel="Create"
      fields={[
        { name: "pool", label: "Pool" },
        { name: "name", label: "Name" },
        { name: "quota", label: "Quota (bytes)", type: "number" },
        { name: "use", label: "Use" },
      ]}
      run={createVolume}
      successMessage="Volume created"
      invalidateKeys={[["os-storage"]]}
    />
    <OSAction
      label="Delete volume"
      mode="fields"
      variant="danger"
      submitLabel="Delete"
      confirmMessage="This will delete the storage volume."
      fields={[
        { name: "pool", label: "Pool" },
        { name: "name", label: "Name" },
        { name: "force", label: "Force", type: "checkbox" },
      ]}
      run={deleteVolume}
      successMessage="Volume deleted"
      invalidateKeys={[["os-storage"]]}
    />
    <OSAction
      label="Delete pool"
      mode="fields"
      variant="danger"
      submitLabel="Delete"
      confirmMessage="This will delete the storage pool."
      fields={[{ name: "name", label: "Name" }]}
      run={deletePool}
      successMessage="Pool deleted"
      invalidateKeys={[["os-storage"]]}
    />
    <OSAction
      label="Scrub pool"
      mode="fields"
      submitLabel="Scrub"
      fields={[{ name: "name", label: "Name" }]}
      run={scrubPool}
      successMessage="Pool scrub triggered"
    />
    <OSAction
      label="Encrypt drive"
      mode="fields"
      variant="danger"
      submitLabel="Encrypt"
      confirmMessage="This will wipe and encrypt the drive."
      fields={[
        { name: "id", label: "Drive ID" },
        { name: "secure_wipe", label: "Secure wipe", type: "checkbox" },
      ]}
      run={encryptDrive}
      successMessage="Drive encryption triggered"
      invalidateKeys={[["os-storage"]]}
    />
    <OSAction
      label="Wipe drive"
      mode="fields"
      variant="danger"
      submitLabel="Wipe"
      confirmMessage="This will wipe the drive."
      fields={[
        { name: "id", label: "Drive ID" },
        { name: "secure_wipe", label: "Secure wipe", type: "checkbox" },
      ]}
      run={wipeDrive}
      successMessage="Drive wipe triggered"
      invalidateKeys={[["os-storage"]]}
    />
    <OSAction
      label="Import encrypted drive"
      mode="fields"
      submitLabel="Import"
      fields={[
        { name: "id", label: "Drive ID" },
        { name: "key", label: "Encryption key" },
      ]}
      run={importEncryptedDrive}
      successMessage="Drive imported"
      invalidateKeys={[["os-storage"]]}
    />
    <OSAction
      label="Import pool"
      mode="fields"
      submitLabel="Import"
      fields={[
        { name: "name", label: "Name" },
        { name: "encryption_key", label: "Encryption key" },
      ]}
      run={importPool}
      successMessage="Pool imported"
      invalidateKeys={[["os-storage"]]}
    />
  </>
);

const UpdateActions: FC = () => (
  <OSAction
    label="Check for updates"
    mode="confirm"
    confirmMessage="Check for updates and apply any pending update?"
    run={() => runOSAction("system/update", "check", {})}
    successMessage="Update check triggered"
  />
);

const subTabs = [
  {
    key: "network",
    title: "Network",
    content: (
      <OSConfigSection
        endpoint="system/network"
        queryKey="os-network"
        label="Network"
        actions={<NetworkActions />}
      />
    ),
  },
  {
    key: "storage",
    title: "Storage",
    content: (
      <OSConfigSection
        endpoint="system/storage"
        queryKey="os-storage"
        label="Storage"
        actions={<StorageActions />}
      />
    ),
  },
  {
    key: "security",
    title: "Security",
    content: (
      <OSConfigSection
        endpoint="system/security"
        queryKey="os-security"
        label="Security"
        actions={<SecurityActions />}
      />
    ),
  },
  {
    key: "update",
    title: "Update",
    content: (
      <OSConfigSection
        endpoint="system/update"
        queryKey="os-system-update"
        label="Update"
        actions={<UpdateActions />}
      />
    ),
  },
  {
    key: "kernel",
    title: "Kernel",
    content: (
      <OSConfigSection
        endpoint="system/kernel"
        queryKey="os-kernel"
        label="Kernel"
      />
    ),
  },
  {
    key: "logging",
    title: "Logging",
    content: (
      <OSConfigSection
        endpoint="system/logging"
        queryKey="os-logging"
        label="Logging"
      />
    ),
  },
  {
    key: "provider",
    title: "Provider",
    content: (
      <OSConfigSection
        endpoint="system/provider"
        queryKey="os-provider"
        label="Provider"
      />
    ),
  },
  {
    key: "fallback-listener",
    title: "Fallback listener",
    content: (
      <OSConfigSection
        endpoint="system/fallback-listener"
        queryKey="os-fallback-listener"
        label="Fallback listener"
      />
    ),
  },
  {
    key: "resources",
    title: "Resources",
    content: (
      <OSConfigSection
        endpoint="system/resources"
        queryKey="os-resources"
        label="Resources"
        readOnly
      />
    ),
  },
].sort((a, b) => a.title.localeCompare(b.title));

const OSSystem: FC = () => {
  const navigate = useNavigate();
  const { subTab } = useParams<{ subTab?: string }>();

  return (
    <TabView
      defaultTab={subTabs[0].key}
      activeTab={subTab}
      tabs={subTabs}
      onSelect={(key) => navigate(`/ui/os/system/${key}`)}
    />
  );
};

export default OSSystem;
