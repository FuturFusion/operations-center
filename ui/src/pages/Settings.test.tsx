import { screen, waitFor, within } from "@testing-library/react";
import { Route, Routes } from "react-router";
import { expect, test } from "vitest";
import Settings from "pages/Settings";
import {
  apiError,
  Handler,
  mockFetch,
  networkError,
  ok,
  Routes as FetchRoutes,
} from "test/fetch";
import { renderWithProviders } from "test/render";
import { SystemSecurity } from "types/settings";

const getSecurity = "GET /1.0/system/security";
const putSecurity = "PUT /1.0/system/security";
const getNetwork = "GET /1.0/system/network";

const securityWithoutOIDC: SystemSecurity = {
  trusted_tls_client_cert_fingerprints: [],
  trusted_tls_client_certificates: [],
  trusted_tls_metrics_client_cert_fingerprints: [],
  trusted_tls_metrics_client_certificates: [],
  trusted_https_proxies: [],
  oidc: { issuer: "", client_id: "", scopes: "", audience: "", claim: "" },
  openfga: { api_token: "", api_url: "", store_id: "" },
  acme: {
    agree_tos: false,
    ca_url: "https://acme-v02.api.letsencrypt.org/directory",
    challenge: "HTTP-01",
    domain: "",
    email: "",
    http_challenge_address: ":80",
    provider: "",
    provider_environment: [],
    provider_resolvers: [],
  },
};

const securityWithOIDC: SystemSecurity = {
  ...securityWithoutOIDC,
  oidc: {
    ...securityWithoutOIDC.oidc,
    issuer: "https://idp.example.com",
    client_id: "operations-center",
  },
};

// securityServer stores the security settings of each PUT and returns them on GET, like the real server.
const securityServer = (initial: SystemSecurity): FetchRoutes => {
  let stored = initial;

  return {
    [getSecurity]: (request) => ok(stored)(request),
    [putSecurity]: (request) => {
      stored = JSON.parse(request.body ?? "");
      return ok()(request);
    },
    [getNetwork]: ok({ address: "", rest_server_address: "" }),
  };
};

// sequence answers the nth request with the nth handler and repeats the last one.
const sequence = (...handlers: Handler[]): Handler => {
  let next = 0;

  return (request) => {
    const handler = handlers[Math.min(next, handlers.length - 1)];
    next++;
    return handler(request);
  };
};

const renderSettings = (tab: string, queryData: [string[], unknown][] = []) => {
  return renderWithProviders(
    <Routes>
      <Route path="/ui/settings/:activeTab" element={<Settings />} />
    </Routes>,
    { route: `/ui/settings/${tab}`, queryData },
  );
};

const lastBody = (api: ReturnType<typeof mockFetch>, key: string) => {
  const calls = api.calls(key);
  return JSON.parse(calls[calls.length - 1]?.body ?? "");
};

test("security keeps saved OIDC settings after a tab switch", async () => {
  const api = mockFetch(securityServer(securityWithoutOIDC));
  const view = renderSettings("security");

  const oidc = within(await screen.findByRole("group", { name: "OIDC" }));
  await view.user.type(
    oidc.getByLabelText("Issuer URL"),
    "https://idp.example.com",
  );
  await view.user.type(oidc.getByLabelText("Client ID"), "operations-center");
  await view.user.click(screen.getByRole("button", { name: "Submit" }));
  await screen.findByText("System security updated");

  await view.user.click(screen.getByRole("tab", { name: "Network" }));
  await screen.findByLabelText("REST API listen address");
  await view.user.click(screen.getByRole("tab", { name: "Security" }));

  await view.user.type(
    await screen.findByLabelText("Trusted TLS client certificate fingerprints"),
    "abc123",
  );
  await view.user.click(screen.getByRole("button", { name: "Submit" }));
  await waitFor(() => {
    expect(
      api.calls(getSecurity),
      "Expect the initial load and one reload per save",
    ).toHaveLength(3);
  });

  expect(api.calls(putSecurity), "Expect one PUT per save").toHaveLength(2);
  expect(
    lastBody(api, putSecurity),
    "Expect the second save to keep the OIDC settings of the first save",
  ).toEqual({
    ...securityWithOIDC,
    trusted_tls_client_cert_fingerprints: ["abc123"],
  });
});

test("security uses fresh data from a background refetch", async () => {
  const api = mockFetch(securityServer(securityWithOIDC));
  const view = renderSettings("security", [
    [["system_security"], securityWithoutOIDC],
  ]);

  const fingerprints = await screen.findByLabelText(
    "Trusted TLS client certificate fingerprints",
  );
  await view.queryClient.refetchQueries({ queryKey: ["system_security"] });
  expect(
    api.calls(getSecurity),
    "Expect only the background refetch to load the settings",
  ).toHaveLength(1);

  await view.user.type(fingerprints, "abc123");
  await view.user.click(screen.getByRole("button", { name: "Submit" }));
  await screen.findByText("System security updated");

  expect(
    lastBody(api, putSecurity),
    "Expect the save to contain the OIDC settings of the refetch",
  ).toEqual({
    ...securityWithOIDC,
    trusted_tls_client_cert_fingerprints: ["abc123"],
  });
});

test("security keeps in-progress edits when a refetch returns unchanged data", async () => {
  const api = mockFetch(securityServer(securityWithOIDC));
  const view = renderSettings("security");

  const fingerprints = await screen.findByLabelText(
    "Trusted TLS client certificate fingerprints",
  );
  await view.user.type(fingerprints, "abc123");
  await view.queryClient.refetchQueries({ queryKey: ["system_security"] });
  expect(
    api.calls(getSecurity),
    "Expect the initial load and the background refetch",
  ).toHaveLength(2);

  await view.user.click(screen.getByRole("button", { name: "Submit" }));
  await screen.findByText("System security updated");

  expect(
    lastBody(api, putSecurity),
    "Expect the save to contain the edit made before the refetch",
  ).toEqual({
    ...securityWithOIDC,
    trusted_tls_client_cert_fingerprints: ["abc123"],
  });
});

test("security resets unsaved edits when a refetch returns changed data", async () => {
  mockFetch({
    [getSecurity]: sequence(ok(securityWithoutOIDC), ok(securityWithOIDC)),
  });
  const view = renderSettings("security");

  const fingerprints = await screen.findByLabelText(
    "Trusted TLS client certificate fingerprints",
  );
  await view.user.type(fingerprints, "abc123");
  await view.queryClient.refetchQueries({ queryKey: ["system_security"] });

  const oidc = within(screen.getByRole("group", { name: "OIDC" }));
  await waitFor(() => {
    expect(
      oidc.getByLabelText("Issuer URL"),
      "Expect the form to show the changed server data",
    ).toHaveValue("https://idp.example.com");
  });
  expect(
    fingerprints,
    "Expect the refetch to drop the unsaved edit",
  ).toHaveValue("");
});

test("security shows the load error when the reload after a save fails", async () => {
  mockFetch({
    [getSecurity]: sequence(
      ok(securityWithOIDC),
      apiError(500, "Reload failed"),
    ),
    [putSecurity]: ok(),
  });
  const view = renderSettings("security");

  await view.user.click(await screen.findByRole("button", { name: "Submit" }));
  await screen.findByText("System security updated");

  expect(
    await screen.findByText(
      "Error while loading the security settings: Reload failed",
    ),
    "Expect the failed reload to show the load error",
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "Submit" }),
    "Expect the load error to replace the form",
  ).not.toBeInTheDocument();
});

const reloadCases: {
  tab: string;
  get: string;
  put: string;
  data: unknown;
  reloaded: unknown;
  field: string;
  value: string;
  other: FetchRoutes;
  message: string;
}[] = [
  {
    tab: "security",
    get: getSecurity,
    put: putSecurity,
    data: securityWithOIDC,
    reloaded: {
      ...securityWithOIDC,
      trusted_tls_client_cert_fingerprints: ["reloaded"],
    },
    field: "Trusted TLS client certificate fingerprints",
    value: "reloaded",
    other: {},
    message: "System security updated",
  },
];

test.each(reloadCases)(
  "$tab reloads its settings after a save",
  async ({ tab, get, put, data, reloaded, field, value, other, message }) => {
    mockFetch({
      [get]: sequence(ok(data), ok(reloaded)),
      [put]: ok(),
      ...other,
    });
    const view = renderSettings(tab);

    await view.user.click(
      await screen.findByRole("button", { name: "Submit" }),
    );
    await screen.findByText(message);

    await waitFor(() => {
      expect(
        screen.getByLabelText(field),
        "Expect the form to show the reloaded settings",
      ).toHaveValue(value);
    });
  },
);

const saveFailures: { failure: string; handler: Handler; text: RegExp }[] = [
  {
    failure: "an API error",
    handler: apiError(500, "Save failed"),
    text: /^Save failed$/,
  },
  {
    failure: "a network error",
    handler: networkError(),
    text: /^Error during .* update: TypeError: Failed to fetch$/,
  },
];

test.each(
  reloadCases.flatMap((reloadCase) =>
    saveFailures.map((saveFailure) => ({ ...reloadCase, ...saveFailure })),
  ),
)(
  "$tab keeps its settings after a save with $failure",
  async ({ tab, get, put, data, other, handler, text }) => {
    const api = mockFetch({ [get]: ok(data), [put]: handler, ...other });
    const view = renderSettings(tab);

    await view.user.click(
      await screen.findByRole("button", { name: "Submit" }),
    );
    await screen.findByText(text);

    expect(
      api.calls(get),
      "Expect a failed save to keep the cached settings",
    ).toHaveLength(1);
    expect(
      screen.getByRole("button", { name: "Submit" }),
      "Expect the form to stay visible",
    ).toBeInTheDocument();
  },
);
