import { vi } from "vitest";

export interface RecordedRequest {
  method: string;
  // Path and query, e.g. "/1.0/provisioning/servers?recursion=1".
  url: string;
  path: string;
  body: string | null;
}

export type Handler = (
  request: RecordedRequest,
) => Response | Promise<Response>;

// Keyed by method and path without the query, e.g. "POST /1.0/provisioning/servers".
export type Routes = Record<string, Handler>;

const unhandled: string[] = [];

export const ok = (metadata: unknown = null): Handler => {
  return () =>
    Response.json({
      type: "sync",
      status: "Success",
      status_code: 200,
      error: "",
      error_code: 0,
      metadata,
    });
};

export const apiError = (
  code: number,
  message: string,
  metadata: unknown = null,
): Handler => {
  return () =>
    Response.json(
      {
        type: "error",
        status: "",
        status_code: 0,
        error: message,
        error_code: code,
        metadata,
      },
      { status: code },
    );
};

export const raw = (status: number, body: string): Handler => {
  return () => new Response(body, { status });
};

export const networkError = (): Handler => {
  return () => {
    throw new TypeError("Failed to fetch");
  };
};

// mockFetch replaces fetch with the given routes for the current test.
export const mockFetch = (routes: Routes) => {
  const requests: RecordedRequest[] = [];

  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = new URL(
      input instanceof Request ? input.url : input.toString(),
      "http://localhost",
    );
    const method = (
      init?.method ?? (input instanceof Request ? input.method : "GET")
    ).toUpperCase();
    const request = {
      method,
      url: url.pathname + url.search,
      path: url.pathname,
      body: typeof init?.body == "string" ? init.body : null,
    };

    requests.push(request);

    const key = `${method} ${url.pathname}`;
    const handler = routes[key];
    if (!handler) {
      unhandled.push(key);
      throw new TypeError(`No mocked response for ${key}`);
    }

    return handler(request);
  });

  return {
    // calls returns the requests made to the route, all requests without one.
    calls: (key?: string) =>
      key === undefined
        ? requests
        : requests.filter((r) => `${r.method} ${r.path}` == key),
  };
};

// takeUnhandledRequests returns and resets the requests without a mocked response.
export const takeUnhandledRequests = (): string[] => {
  return unhandled.splice(0);
};
