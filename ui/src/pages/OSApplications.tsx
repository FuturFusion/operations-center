import type { FC } from "react";
import { Link, useParams } from "react-router";
import {
  MdOutlineBackup,
  MdOutlineChangeCircle,
  MdOutlineDeleteForever,
  MdOutlineRestartAlt,
  MdOutlineRestore,
  MdOutlineSettingsBackupRestore,
  MdOutlineUpdate,
} from "react-icons/md";
import { useQuery } from "@tanstack/react-query";
import {
  addOSApplication,
  fetchOSApplication,
  fetchOSApplications,
  runOSAction,
  runOSActionDownload,
  runOSActionUpload,
} from "api/os";
import ActionGroup from "components/ActionGroup";
import ExtendedDataTable from "components/ExtendedDataTable";
import OSAction from "components/OSAction";
import type { OSActionInput } from "components/OSAction";
import { nameFromURL } from "util/os";
import OSApplicationDetails from "./OSApplicationDetails";

const OSApplications: FC = () => {
  const { subTab } = useParams<{ subTab?: string }>();

  const { data: appUrls } = useQuery({
    queryKey: ["os-apps"],
    queryFn: async () => fetchOSApplications(),
  });

  const {
    data: apps,
    isLoading,
    error,
  } = useQuery({
    queryKey: ["os-apps-details", appUrls],
    queryFn: async () => {
      return Promise.all(
        appUrls?.map(async (url: string) => {
          const res = await fetchOSApplication(url);
          return { name: nameFromURL(url), data: res };
        }) || [],
      );
    },
    enabled: !!appUrls,
  });

  if (subTab) {
    return <OSApplicationDetails name={subTab} />;
  }

  const headers = ["Name", "Version", "Actions"];

  const rows =
    apps?.map((app) => {
      const name = app.name;
      const endpoint = `applications/${name}`;

      const switchVersion = (input: OSActionInput) =>
        runOSAction(endpoint, "switch-version", {
          version: (input as Record<string, string>).version,
        });

      return {
        cols: [
          {
            content: [
              <Link
                to={`/ui/os/applications/${name}`}
                className="data-table-link"
                title="Application details"
              >
                {name}
              </Link>,
            ],
            sortKey: name,
          },
          {
            content: app.data?.state?.version,
          },
          {
            content: (
              <ActionGroup
                groups={[
                  [
                    <OSAction
                      key="check-update"
                      label="Check for updates"
                      mode="confirm"
                      icon={<MdOutlineUpdate size={22} />}
                      confirmMessage={`Check for updates for the ${name} application?`}
                      run={() => runOSAction(endpoint, "check-update")}
                      successMessage={`Update check triggered for ${name}`}
                    />,
                    <OSAction
                      key="switch-version"
                      label="Switch version"
                      mode="fields"
                      submitLabel="Switch"
                      icon={<MdOutlineChangeCircle size={22} />}
                      confirmMessage={`Switch the running version of ${name}?`}
                      fields={[
                        {
                          name: "version",
                          label: "Version",
                          options: app.data?.state?.available_versions ?? [],
                          defaultValue: app.data?.state?.version,
                        },
                      ]}
                      run={switchVersion}
                      successMessage={`Version switched for ${name}`}
                      invalidateKeys={[["os-apps-details"]]}
                    />,
                    <OSAction
                      key="restart"
                      label="Restart application"
                      mode="confirm"
                      icon={<MdOutlineRestartAlt size={22} />}
                      confirmMessage={`Restart the ${name} application?`}
                      run={() => runOSAction(endpoint, "restart")}
                      successMessage={`Application ${name} restarted`}
                    />,
                  ],
                  [
                    <OSAction
                      key="backup"
                      label="Backup application"
                      mode="download"
                      icon={<MdOutlineBackup size={22} />}
                      filename={`${name}-backup`}
                      confirmMessage={`Download a backup of the ${name} application?`}
                      run={() => runOSActionDownload(endpoint, "backup", {})}
                      successMessage={`Application ${name} backed up`}
                    />,
                    <OSAction
                      key="restore"
                      label="Restore application"
                      mode="upload"
                      icon={<MdOutlineRestore size={22} />}
                      run={(input: OSActionInput) =>
                        runOSActionUpload(endpoint, "restore", input as File)
                      }
                      successMessage={`Application ${name} restored`}
                    />,
                    <OSAction
                      key="factory-reset"
                      label="Factory reset application"
                      mode="confirm"
                      icon={<MdOutlineSettingsBackupRestore size={22} />}
                      confirmMessage={`Factory-reset the ${name} application? This wipes its local configuration.`}
                      run={() => runOSAction(endpoint, "factory-reset", {})}
                      successMessage={`Application ${name} factory reset`}
                    />,
                    <OSAction
                      key="remove"
                      label="Remove application"
                      mode="confirm"
                      icon={<MdOutlineDeleteForever size={22} />}
                      confirmMessage={`Remove the ${name} application?`}
                      run={() => runOSAction(endpoint, "remove")}
                      successMessage={`Application ${name} removed`}
                      invalidateKeys={[["os-apps"], ["os-apps-details"]]}
                    />,
                  ],
                ]}
              />
            ),
          },
        ],
      };
    }) || [];

  return (
    <div className="d-flex flex-column">
      <div className="mb-3">
        <OSAction
          label="Add application"
          mode="fields"
          submitLabel="Add"
          fields={[{ name: "name", label: "Name", required: true }]}
          run={(input) =>
            addOSApplication((input as Record<string, string>).name)
          }
          successMessage="Application added"
          invalidateKeys={[["os-apps"], ["os-apps-details"]]}
        />
      </div>
      <ExtendedDataTable
        headers={headers}
        rows={rows}
        isLoading={isLoading}
        error={error}
      />
    </div>
  );
};

export default OSApplications;
