import type { FC } from "react";
import { useState } from "react";
import { Button, Form } from "react-bootstrap";
import { useQuery } from "@tanstack/react-query";
import { fetchDebugLogs } from "api/os";
import type { DebugLogOptions } from "api/os";
import type { IncusOSLog } from "types/os";

function formatTimestamp(us: string) {
  const ms = Number(us) / 1000;
  return new Date(ms).toLocaleString(undefined, {
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function decodeMessage(message: string | number[]): string {
  // systemd's journal JSON export encodes messages containing
  // non-printable or non-UTF-8 bytes as an array of byte values
  // rather than as a plain string.
  const text = Array.isArray(message)
    ? new TextDecoder().decode(new Uint8Array(message))
    : message;

  // Strip ANSI escape sequences (e.g. color codes) which would
  // otherwise show up as garbage in the browser.
  // eslint-disable-next-line no-control-regex
  return text.replace(/\x1b\[[0-9;]*[A-Za-z]/g, "");
}

function JournalLine(item: IncusOSLog) {
  const ts = formatTimestamp(item.__REALTIME_TIMESTAMP);
  const host = item._HOSTNAME ?? "";
  const ident = item.SYSLOG_IDENTIFIER ?? item._COMM ?? "unknown";
  const pid = item._PID ? `[${item._PID}]` : "";
  const msg = decodeMessage(item.MESSAGE ?? "");

  return (
    <>
      {ts} {host} {ident}
      {pid}: {msg}
      <br />
    </>
  );
}

const OSDebugLog: FC = () => {
  const [unit, setUnit] = useState("");
  const [boot, setBoot] = useState("");
  const [entries, setEntries] = useState("200");
  const [filters, setFilters] = useState<DebugLogOptions>({ entries: 200 });

  const {
    data: logs = [],
    isLoading,
    error,
  } = useQuery({
    queryKey: ["os-debug-logs", filters],
    queryFn: async () => fetchDebugLogs(filters),
  });

  const applyFilters = () => {
    setFilters({
      unit: unit,
      boot: boot,
      entries: entries ? Number(entries) : undefined,
    });
  };

  return (
    <div>
      <Form
        className="d-flex gap-2 align-items-end mb-3 flex-wrap"
        onSubmit={(e) => {
          e.preventDefault();
          applyFilters();
        }}
      >
        <Form.Group>
          <Form.Label className="mb-0">Unit</Form.Label>
          <Form.Control
            size="sm"
            type="text"
            value={unit}
            onChange={(e) => setUnit(e.target.value)}
          />
        </Form.Group>
        <Form.Group>
          <Form.Label className="mb-0">Boot</Form.Label>
          <Form.Control
            size="sm"
            type="text"
            style={{ maxWidth: "120px" }}
            value={boot}
            onChange={(e) => setBoot(e.target.value)}
          />
        </Form.Group>
        <Form.Group>
          <Form.Label className="mb-0">Entries</Form.Label>
          <Form.Control
            size="sm"
            type="number"
            style={{ maxWidth: "120px" }}
            value={entries}
            onChange={(e) => setEntries(e.target.value)}
          />
        </Form.Group>
        <Button size="sm" variant="primary" type="submit">
          Apply
        </Button>
      </Form>

      {error && (
        <div className="u-align-text--center">Error during logs load</div>
      )}
      {!isLoading && logs.length === 0 && (
        <div className="u-align-text--center">There are no logs.</div>
      )}
      {!isLoading && logs.length > 0 && (
        <pre className="bg-light" style={{ width: "80vw" }}>
          {logs?.map((item, i) => <span key={i}>{JournalLine(item)}</span>)}
        </pre>
      )}
    </div>
  );
};

export default OSDebugLog;
