// @vitest-environment node
import { expect, test } from "vitest";
import { APIError, errorMessage, processResponse } from "util/response";
import { APIResponse, ErrorMetadata } from "types/response";

const errorResponse = (
  code: number,
  metadata: ErrorMetadata | null,
): APIResponse<ErrorMetadata | null> => ({
  error: "Something failed",
  error_code: code,
  metadata,
  operation: "",
  status: "",
  status_code: 0,
  type: "error",
});

test.each([
  { code: 400, metadata: null, want: "Something failed" },
  {
    code: 400,
    metadata: { reason: "", request_id: "abc" },
    want: "Something failed",
  },
  {
    code: 500,
    metadata: { reason: "", request_id: "abc" },
    want: "Something failed (request ID abc)",
  },
  {
    code: 404,
    metadata: { reason: "", hint: "Check the name." },
    want: "Something failed Check the name.",
  },
  {
    code: 503,
    metadata: { reason: "", hint: "Retry later.", request_id: "abc" },
    want: "Something failed (request ID abc) Retry later.",
  },
])("errorMessage for $code with $metadata", ({ code, metadata, want }) => {
  expect(errorMessage(errorResponse(code, metadata))).toBe(want);
});

test("processResponse returns the body of a successful response", async () => {
  const response = new Response(JSON.stringify({ metadata: "ok" }), {
    status: 200,
  });

  await expect(processResponse(response)).resolves.toEqual({ metadata: "ok" });
});

test("processResponse throws an APIError for an error response", async () => {
  const body = errorResponse(500, {
    reason: "db",
    hint: "Retry later.",
    details: { key: "value" },
    request_id: "abc",
  });
  const response = new Response(JSON.stringify(body), { status: 500 });

  const error = await processResponse(response).catch((e) => e);

  expect(error).toBeInstanceOf(APIError);
  expect(error.message).toBe("Something failed (request ID abc) Retry later.");
  expect(error.code).toBe(500);
  expect(error.reason).toBe("db");
  expect(error.hint).toBe("Retry later.");
  expect(error.details).toEqual({ key: "value" });
  expect(error.requestId).toBe("abc");
});

test("processResponse throws a plain error for a non JSON body", async () => {
  const response = new Response("<html>" + "x".repeat(300), {
    status: 502,
    statusText: "Bad Gateway",
  });

  const error = await processResponse(response).catch((e) => e);

  expect(error).not.toBeInstanceOf(APIError);
  expect(error.message).toBe("502 Bad Gateway: <html>" + "x".repeat(194));
});
