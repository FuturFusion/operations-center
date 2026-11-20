import { screen, waitFor } from "@testing-library/react";
import { expect, test } from "vitest";
import OSConfigSection from "components/OSConfigSection";
import { apiError, Handler, mockFetch, ok } from "test/fetch";
import { expectToast, renderWithProviders } from "test/render";

const section = "GET /os/1.0/system/network";
const update = "PUT /os/1.0/system/network";

const renderSection = async (updateHandler: Handler) => {
  const api = mockFetch({
    [section]: ok({ config: { dns: { hostname: "host01" } } }),
    [update]: updateHandler,
  });
  const view = renderWithProviders(
    <OSConfigSection
      endpoint="system/network"
      queryKey="network"
      label="Network"
    />,
  );

  await view.user.click(await screen.findByLabelText("Edit mode"));

  return { ...view, api };
};

// The YAML is pasted, since user.type interprets braces and brackets.
const submitYAML = async (
  view: Awaited<ReturnType<typeof renderSection>>,
  yaml: string,
) => {
  const editor = screen.getByRole("textbox");
  await view.user.clear(editor);
  await view.user.click(editor);
  await view.user.paste(yaml);
  await view.user.click(screen.getByRole("button", { name: "Submit" }));
};

test("shows the section as YAML", async () => {
  await renderSection(ok());

  expect(screen.getByRole("textbox")).toHaveValue(
    "config:\n  dns:\n    hostname: host01\n",
  );
});

test("submits the edited config", async () => {
  const view = await renderSection(ok());

  await submitYAML(view, "config:\n  dns:\n    hostname: host02\n");

  await expectToast("Network updated", "success");
  expect(view.api.calls(update)).toHaveLength(1);
  expect(JSON.parse(view.api.calls(update)[0].body ?? "")).toEqual({
    config: { dns: { hostname: "host02" } },
  });
  // The editor closes after a successful update.
  await waitFor(() => {
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });
});

test("rejects invalid YAML without a request", async () => {
  const view = await renderSection(ok());

  await submitYAML(view, "config: [unclosed");

  await expectToast(/Error during YAML value parsing/, "error");
  expect(view.api.calls(update)).toHaveLength(0);
  expect(screen.getByRole("textbox")).toBeInTheDocument();
});

test("keeps the editor open, if the update fails", async () => {
  const view = await renderSection(apiError(400, "Invalid hostname"));

  await submitYAML(view, "config:\n  dns:\n    hostname: bad_name\n");

  await expectToast(
    "Network update failed: APIError: Invalid hostname",
    "error",
  );
  expect(screen.getByRole("textbox")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Submit" })).toBeEnabled();
});
