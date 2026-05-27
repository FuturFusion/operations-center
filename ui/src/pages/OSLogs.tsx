import type { FC } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchDebugLogs } from "api/os";
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

function JournalLine(item: IncusOSLog) {
  const ts = formatTimestamp(item.__REALTIME_TIMESTAMP);
  const host = item._HOSTNAME ?? "";
  const ident = item.SYSLOG_IDENTIFIER ?? item._COMM ?? "unknown";
  const pid = item._PID ? `[${item._PID}]` : "";
  const msg = item.MESSAGE ?? "";

  return (
    <>
      {ts} {host} {ident}
      {pid}: {msg}
      <br />
    </>
  );
}

const OSLogs: FC = () => {
  const entriesLimit = 200;

  const {
    data: logs = [],
    isLoading,
    error,
  } = useQuery({
    queryKey: ["os-debug-logs"],
    queryFn: async () => fetchDebugLogs(entriesLimit),
  });
  return (
    <div>
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

export default OSLogs;
