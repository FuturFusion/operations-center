import { useState } from "react";
import { Badge, Form } from "react-bootstrap";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams } from "react-router";
import { IoChevronDownOutline, IoChevronUpOutline } from "react-icons/io5";
import {
  fetchServer,
  fetchServerBMCLogEntries,
  fetchServerBMCLogSources,
  powerOffServerBMC,
  powerOnServerBMC,
  refreshServerBMC,
  restartServerBMC,
} from "api/server";
import ExtendedDataTable from "components/ExtendedDataTable";
import LoadingButton from "components/LoadingButton";
import OSAction from "components/OSAction";
import type { OSActionField, OSActionValues } from "components/OSAction";
import { useNotification } from "context/notificationContext";
import { formatDate } from "util/date";

const powerStateBadge = (state: string | undefined) => {
  if (!state) {
    return <Badge bg="secondary">Unknown</Badge>;
  }

  if (state == "On") {
    return <Badge bg="success">{state}</Badge>;
  }

  if (state == "Off") {
    return <Badge bg="secondary">{state}</Badge>;
  }

  return <Badge bg="warning">{state}</Badge>;
};

const healthStatusBadge = (status: string | undefined) => {
  if (!status) {
    return "";
  }

  if (status == "OK") {
    return <Badge bg="success">{status}</Badge>;
  }

  if (status == "Warning") {
    return <Badge bg="warning">{status}</Badge>;
  }

  return <Badge bg="danger">{status}</Badge>;
};

const forceField: OSActionField[] = [
  { name: "force", label: "Force", type: "checkbox" },
];

const ServerBMC = () => {
  const { name } = useParams() as { name: string };
  const { notify } = useNotification();
  const queryClient = useQueryClient();
  const [logSource, setLogSource] = useState("");
  const [isDataVisible, setIsDataVisible] = useState(false);
  const [refreshing, setRefreshing] = useState(false);

  const {
    data: server = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["servers", name],
    queryFn: () => fetchServer(name),
  });

  const hasBMC = !!server?.bmc_config?.api_type;

  const {
    data: logSources = [],
    error: logSourcesError,
    isLoading: isLogSourcesLoading,
  } = useQuery({
    queryKey: ["servers", name, "bmc-log-sources"],
    queryFn: () => fetchServerBMCLogSources(name),
    enabled: hasBMC,
  });

  const {
    data: logEntries = [],
    error: logEntriesError,
    isLoading: isLogEntriesLoading,
  } = useQuery({
    queryKey: ["servers", name, "bmc-log-entries", logSource],
    queryFn: () => fetchServerBMCLogEntries(name, logSource),
    enabled: hasBMC && logSource != "",
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading servers</div>;
  }

  if (!hasBMC) {
    return (
      <div>
        No BMC is configured for this server. The BMC connection details can be
        set in the Configuration tab.
      </div>
    );
  }

  const handleRefresh = async () => {
    setRefreshing(true);
    try {
      await refreshServerBMC(name);
      notify.success(`BMC data for ${name} refreshed`);
      queryClient.invalidateQueries({ queryKey: ["servers", name] });
    } catch (e) {
      notify.error(`Error during BMC data refresh: ${e}`);
    }
    setRefreshing(false);
  };

  const bmcData = server?.bmc_data;

  const logRows = logEntries.map((entry) => {
    return {
      cols: [
        { content: entry.Timestamp, sortKey: entry.Timestamp },
        { content: entry.Severity, sortKey: entry.Severity },
        { content: entry.EntryType, sortKey: entry.EntryType },
        { content: entry.EntryCode, sortKey: entry.EntryCode },
        { content: entry.Message },
      ],
    };
  });

  return (
    <div className="container">
      <div className="d-flex justify-content-end gap-2 mb-3">
        <LoadingButton
          isLoading={refreshing}
          size="sm"
          variant="success"
          onClick={handleRefresh}
        >
          Refresh
        </LoadingButton>
        <OSAction
          label="Power on"
          mode="fields"
          confirmMessage={`Power on the server "${name}" via its BMC?`}
          fields={forceField}
          run={(input) =>
            powerOnServerBMC(name, Boolean((input as OSActionValues).force))
          }
          successMessage="Server power on triggered"
          invalidateKeys={[["servers", name]]}
        />
        <OSAction
          label="Power off"
          mode="fields"
          variant="danger"
          confirmMessage={`Power off the server "${name}" via its BMC?`}
          fields={forceField}
          run={(input) =>
            powerOffServerBMC(name, Boolean((input as OSActionValues).force))
          }
          successMessage="Server power off triggered"
          invalidateKeys={[["servers", name]]}
        />
        <OSAction
          label="Restart"
          mode="fields"
          variant="danger"
          confirmMessage={`Restart the server "${name}" via its BMC?`}
          fields={forceField}
          run={(input) =>
            restartServerBMC(name, Boolean((input as OSActionValues).force))
          }
          successMessage="Server restart triggered"
          invalidateKeys={[["servers", name]]}
        />
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Power state</div>
        <div className="col-10 detail-table-cell">
          {powerStateBadge(bmcData?.server_power_state)}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Health status</div>
        <div className="col-10 detail-table-cell">
          {healthStatusBadge(bmcData?.server_health_status)}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">BMC vendor</div>
        <div className="col-10 detail-table-cell">{bmcData?.bmc_vendor}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">BMC model</div>
        <div className="col-10 detail-table-cell">{bmcData?.bmc_model}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">BMC firmware version</div>
        <div className="col-10 detail-table-cell">
          {bmcData?.bmc_firmware_version}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Protocol</div>
        <div className="col-10 detail-table-cell">
          {bmcData?.bmc_protocol} {bmcData?.bmc_protocol_version}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Server manufacturer</div>
        <div className="col-10 detail-table-cell">
          {bmcData?.server_manufacturer}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Server model</div>
        <div className="col-10 detail-table-cell">{bmcData?.server_model}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Serial number</div>
        <div className="col-10 detail-table-cell">
          {bmcData?.server_serial_number}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">BIOS version</div>
        <div className="col-10 detail-table-cell">
          {bmcData?.server_bios_version}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Last updated</div>
        <div className="col-10 detail-table-cell">
          {formatDate(bmcData?.last_updated || "")}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">
          BMC data{" "}
          <span
            onClick={() => setIsDataVisible(!isDataVisible)}
            className="hide-field-switch"
          >
            {isDataVisible ? (
              <>
                <IoChevronDownOutline /> Hide
              </>
            ) : (
              <>
                <IoChevronUpOutline /> Show
              </>
            )}
          </span>
        </div>
        <div className="col-10 detail-table-cell">
          {isDataVisible && <pre>{JSON.stringify(bmcData, null, 2)}</pre>}
        </div>
      </div>
      <h5 className="mt-4">Event logs</h5>
      {logSourcesError ? (
        <div>
          Error while loading log sources: <pre>{logSourcesError.message}</pre>
        </div>
      ) : (
        <>
          <Form.Select
            className="mb-3 w-auto"
            value={logSource}
            disabled={isLogSourcesLoading}
            onChange={(e) => setLogSource(e.target.value)}
          >
            <option value="">Select a log source</option>
            {logSources.map((source) => (
              <option key={source} value={source}>
                {source}
              </option>
            ))}
          </Form.Select>
          {logSource != "" && (
            <ExtendedDataTable
              headers={["Timestamp", "Severity", "Type", "Code", "Message"]}
              rows={logRows}
              isLoading={isLogEntriesLoading}
              error={logEntriesError}
            />
          )}
        </>
      )}
    </div>
  );
};

export default ServerBMC;
