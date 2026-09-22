import { APIResponse, ErrorMetadata } from "types/response";

// APIError is an error response of the API.
export class APIError extends Error {
  readonly code: number;
  readonly reason: string;
  readonly hint: string;
  readonly details: Record<string, string>;
  readonly requestId: string;

  constructor(response: APIResponse<ErrorMetadata | null>) {
    super(errorMessage(response));

    this.name = "APIError";
    this.code = response.error_code;
    this.reason = response.metadata?.reason ?? "";
    this.hint = response.metadata?.hint ?? "";
    this.details = response.metadata?.details ?? {};
    this.requestId = response.metadata?.request_id ?? "";
  }
}

// errorMessage returns the message shown to the user. The hint of the server,
// which tells the user how to resolve the error, is appended, if the server
// reports one. For an error caused by the server itself, the ID of the request
// is added, it allows to find the corresponding records in the log of the
// server.
export const errorMessage = <T>(response: APIResponse<T>): string => {
  const metadata = response.metadata as ErrorMetadata | null;
  const requestId = metadata?.request_id ?? "";
  const hint = metadata?.hint ?? "";

  let message = response.error;

  if (response.error_code >= 500 && requestId !== "") {
    message = `${message} (request ID ${requestId})`;
  }

  if (hint !== "") {
    message = `${message} ${hint}`;
  }

  return message;
};

export const processResponse = async (response: Response) => {
  if (!response.ok) {
    const error = (await response.json()) as APIResponse<ErrorMetadata | null>;
    throw new APIError(error);
  }
  return response.json();
};
