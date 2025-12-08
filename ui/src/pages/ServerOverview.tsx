import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router";
import { IoChevronDownOutline, IoChevronUpOutline } from "react-icons/io5";
import { fetchServer } from "api/server";
import { formatDate } from "util/date";
import type { ServerTypeKey } from "util/server";
import { ServerTypeString } from "util/server";

type FieldState = {
  hw: boolean;
  os: boolean;
};

const ServerOverview = () => {
  const { name } = useParams();
  const [isVisible, setIsVisible] = useState<FieldState>({
    hw: false,
    os: false,
  });

  const toogleData = (field: keyof FieldState) => {
    setIsVisible({ ...isVisible, [field]: !isVisible[field] });
  };

  const hideFieldSwitch = (field: keyof FieldState) => {
    return (
      <span onClick={() => toogleData(field)} className="hide-field-switch">
        {isVisible[field] ? (
          <>
            <IoChevronDownOutline /> Hide
          </>
        ) : (
          <>
            <IoChevronUpOutline /> Show
          </>
        )}
      </span>
    );
  };

  const {
    data: server = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["tokens", name],
    queryFn: () => fetchServer(name || ""),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading servers</div>;
  }

  return (
    <div className="container">
      <div className="row">
        <div className="col-2 detail-table-header">Cluster name</div>
        <div className="col-10 detail-table-cell">{server?.cluster}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Name</div>
        <div className="col-10 detail-table-cell">{server?.name}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Connection URL</div>
        <div className="col-10 detail-table-cell">
          <Link
            to={`${server?.connection_url}`}
            target="_blank"
            className="data-table-link"
          >
            {server?.connection_url}
          </Link>
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Type</div>
        <div className="col-10 detail-table-cell">
          {ServerTypeString[(server?.server_type as ServerTypeKey) ?? ""]}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Status</div>
        <div className="col-10 detail-table-cell">{server?.server_status}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Certificate</div>
        <div className="col-10 detail-table-cell">
          <pre>{server?.certificate}</pre>
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Fingerprint</div>
        <div className="col-10 detail-table-cell">{server?.fingerprint}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Last updated</div>
        <div className="col-10 detail-table-cell">
          {formatDate(server?.last_updated || "")}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Last seen</div>
        <div className="col-10 detail-table-cell">
          {formatDate(server?.last_seen || "")}
        </div>
      </div>
      {server?.hardware_data && (
        <div className="row">
          <div className="col-2 detail-table-header">
            Hardware data {hideFieldSwitch("hw")}
          </div>
          <div className="col-10 detail-table-cell">
            {isVisible["hw"] && (
              <pre>{JSON.stringify(server?.hardware_data, null, 2)}</pre>
            )}
          </div>
        </div>
      )}
      {server?.os_data && (
        <div className="row">
          <div className="col-2 detail-table-header">
            OS data {hideFieldSwitch("os")}
          </div>
          <div className="col-10 detail-table-cell">
            {isVisible["os"] && (
              <pre>{JSON.stringify(server?.os_data, null, 2)}</pre>
            )}
          </div>
        </div>
      )}
      {server?.version_data && (
        <div className="row">
          <div className="col-2 detail-table-header">Version data</div>
          <div className="col-10 detail-table-cell">
            <pre>{JSON.stringify(server?.version_data, null, 2)}</pre>
          </div>
        </div>
      )}
    </div>
  );
};

export default ServerOverview;
