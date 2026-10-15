import { FC } from "react";
import { APIError } from "util/response";

interface Props {
  error: Error;
  label: string;
}

// QueryError shows why loading the data failed. A missing permission gets a
// dedicated message, because the user cannot fix it by retrying. A 403 can
// also mean that OpenFGA is not reachable or outdated. So the message of the
// server is always shown.
const QueryError: FC<Props> = ({ error, label }) => {
  if (error instanceof APIError && error.code === 403) {
    return (
      <div>
        You do not have permission to view {label}. This requires the admin
        role. Server response: {error.message}
      </div>
    );
  }

  return (
    <div>
      Error while loading {label}: {error.message}
    </div>
  );
};

export default QueryError;
