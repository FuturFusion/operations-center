import { APIResponse, ErrorMetadata } from "types/response";

// APIError is an error response of the API.
export class APIError extends Error {
  readonly code: number;
  readonly reason: string;
  readonly details: Record<string, string>;
  readonly requestId: string;

  constructor(response: APIResponse<ErrorMetadata | null>) {
    super(errorMessage(response));

    this.name = "APIError";
    this.code = response.error_code;
    this.reason = response.metadata?.reason ?? "";
    this.details = response.metadata?.details ?? {};
    this.requestId = response.metadata?.request_id ?? "";
  }
}

// errorMessage returns the message shown to the user. For an error caused by
// the server itself, the ID of the request is added, it allows to find the
// corresponding records in the log of the server.
export const errorMessage = (
  response: APIResponse<ErrorMetadata | null>,
): string => {
  const requestId = response.metadata?.request_id ?? "";

  if (response.error_code >= 500 && requestId !== "") {
    return `${response.error} (request ID ${requestId})`;
  }

  return response.error;
};

export const processResponse = async (response: Response) => {
  if (!response.ok) {
    const error = (await response.json()) as APIResponse<ErrorMetadata | null>;
    throw new APIError(error);
  }
  return response.json();
};
