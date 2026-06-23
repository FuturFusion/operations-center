import type { FC } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchDebugProcesses } from "api/os";

const OSDebugProcesses: FC = () => {
  const {
    data: processes = "",
    isLoading,
    error,
  } = useQuery({
    queryKey: ["os-debug-processes"],
    queryFn: async () => fetchDebugProcesses(),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return (
      <div className="u-align-text--center">Error during process load</div>
    );
  }

  return (
    <pre className="bg-light" style={{ width: "80vw" }}>
      {processes}
    </pre>
  );
};

export default OSDebugProcesses;
