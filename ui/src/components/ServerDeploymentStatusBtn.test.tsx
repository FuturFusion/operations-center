import { screen } from "@testing-library/react";
import { expect, test } from "vitest";
import ServerDeploymentStatusBtn from "components/ServerDeploymentStatusBtn";
import { apiError, Handler, mockFetch, ok } from "test/fetch";
import { renderWithProviders } from "test/render";
import { Server } from "types/server";

const getServer = "GET /1.0/provisioning/servers/server01";

const deployment = {
  state: "wait-install",
  failed_state: "",
  last_error: "",
  retries: 0,
  started_at: "2026-11-26T08:00:00",
  state_entered_at: "2026-11-26T08:05:00",
  finished_at: "",
  request: {
    token_uuid: "token01",
    seed: "seed01",
    virtual_media_id: "cd",
    skip_secure_boot_certificates: false,
    secure_boot_enrollment_media: false,
  },
  force_reboot: true,
  media_url: "",
  secure_boot_media_url: "",
  media_bytes_read: -1,
  bios_profiles: [],
  bios_attributes: {},
  bios_deferred_attributes: {},
  history: [],
};

const openDialogWith = async (serverHandler: Handler) => {
  const api = mockFetch({ [getServer]: serverHandler });
  const listEntry = {
    name: "server01",
    deployment_state: "wait-install",
  } as Server;
  const view = renderWithProviders(
    <ServerDeploymentStatusBtn server={listEntry} />,
  );

  expect(api.calls(getServer)).toHaveLength(0);

  await view.user.click(screen.getByTitle("Show deployment status"));

  return api;
};

test("no icon without a deployment", () => {
  const api = mockFetch({});
  const listEntry = { name: "server01", deployment_state: "" } as Server;

  renderWithProviders(<ServerDeploymentStatusBtn server={listEntry} />);

  expect(screen.queryByTitle("Show deployment status")).not.toBeInTheDocument();
  expect(api.calls()).toHaveLength(0);
});

test("dialog loads the deployment status of the server", async () => {
  const api = await openDialogWith(ok({ name: "server01", deployment }));

  expect(await screen.findByText("wait-install")).toBeInTheDocument();
  expect(screen.getByText("token01")).toBeInTheDocument();
  expect(api.calls(getServer)).toHaveLength(1);
});

test("dialog shows an error if the server can not be loaded", async () => {
  const api = await openDialogWith(apiError(500, "Database is down"));

  expect(
    await screen.findByText(
      /Failed to load the deployment status:.*Database is down/,
    ),
  ).toBeInTheDocument();
  expect(api.calls(getServer)).toHaveLength(1);
});

test("dialog explains a server without a deployment status", async () => {
  const api = await openDialogWith(ok({ name: "server01" }));

  expect(
    await screen.findByText(
      "No deployment status is available for this server.",
    ),
  ).toBeInTheDocument();
  expect(api.calls(getServer)).toHaveLength(1);
});
