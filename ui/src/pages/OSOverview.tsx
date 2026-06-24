import type { FC } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  fetchOS,
  fetchOSApplication,
  fetchOSApplications,
  fetchSystemUpdate,
} from "api/os";
import { nameFromURL } from "util/os";
import YAML from "yaml";

const OSOverview: FC = () => {
  const { data: incusOSData } = useQuery({
    queryKey: ["os"],
    queryFn: () => fetchOS(),
  });

  const { data: systemUpdate } = useQuery({
    queryKey: ["os-update"],
    queryFn: async () => fetchSystemUpdate(),
  });

  const { data: appUrls } = useQuery({
    queryKey: ["os-apps"],
    queryFn: async () => fetchOSApplications(),
  });

  const apps = useQuery({
    queryKey: ["os-apps-details"],
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

  return (
    <>
      <h6 className="mb-3">General</h6>
      <div className="container">
        <div className="row">
          <div className="col-2 detail-table-header">Hostname</div>
          <div className="col-10 detail-table-cell">
            {incusOSData?.environment?.hostname}
          </div>
        </div>
        <div className="row">
          <div className="col-2 detail-table-header">OS name</div>
          <div className="col-10 detail-table-cell">
            {incusOSData?.environment?.os_name}
          </div>
        </div>
        <div className="row">
          <div className="col-2 detail-table-header">Version</div>
          <div className="col-10 detail-table-cell">
            {incusOSData?.environment?.os_version}
          </div>
        </div>
        <div className="row">
          <div className="col-2 detail-table-header">Update status</div>
          <div className="col-10 detail-table-cell">
            {systemUpdate?.state?.status}
            {systemUpdate?.state?.needs_reboot && " (reboot required)"}
          </div>
        </div>
        <div className="row">
          <div className="col-2 detail-table-header">
            Installed applications
          </div>
          <div className="col-10 detail-table-cell">
            {apps.data?.map((app) => (
              <div key={app.name}>
                {app.name} {app.data?.state?.version}
              </div>
            ))}
          </div>
        </div>
      </div>

      <h6 className="mb-3 mt-4">System details</h6>
      <pre className="bg-light border rounded-3 p-3 mb-0 yaml-editor">
        {YAML.stringify(incusOSData, null, 2)}
      </pre>
    </>
  );
};

export default OSOverview;
