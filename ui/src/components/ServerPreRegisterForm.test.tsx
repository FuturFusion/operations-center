import { screen, waitFor } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import ServerPreRegisterForm from "components/ServerPreRegisterForm";
import { mockFetch, ok } from "test/fetch";
import { renderWithProviders } from "test/render";

const renderForm = () => {
  mockFetch({
    "GET /1.0/provisioning/channels": ok([
      { name: "stable" },
      { name: "testing" },
    ]),
  });
  const handleSubmit = vi.fn().mockResolvedValue(undefined);
  const view = renderWithProviders(
    <ServerPreRegisterForm onSubmit={handleSubmit} />,
  );

  return { ...view, handleSubmit };
};

test("requires a name", async () => {
  const { user, handleSubmit } = renderForm();

  await user.click(screen.getByRole("button", { name: "Pre register" }));

  expect(await screen.findByText("Name is required")).toBeInTheDocument();
  expect(handleSubmit).not.toHaveBeenCalled();
});

test("submits the entered values", async () => {
  const { user, handleSubmit } = renderForm();

  await user.type(screen.getByLabelText("Name"), "server01");
  await user.type(
    screen.getByLabelText("Connection URL"),
    "https://server01:8443",
  );
  await user.selectOptions(
    await screen.findByRole("combobox"),
    await screen.findByRole("option", { name: "testing" }),
  );
  await user.type(screen.getByLabelText("Username"), "admin");
  await user.click(screen.getByRole("button", { name: "Pre register" }));

  await waitFor(() => {
    expect(handleSubmit).toHaveBeenCalledOnce();
  });
  expect(handleSubmit.mock.calls[0][0]).toEqual({
    name: "server01",
    description: "",
    properties: {},
    public_connection_url: "https://server01:8443",
    channel: "testing",
    bmc_endpoint: "",
    bmc_certificate: "",
    bmc_auto_pin_certificate: false,
    bmc_username: "admin",
    bmc_password: "",
  });
});

test("disables recording the certificate, if one is given", async () => {
  const { user } = renderForm();
  const autoPin = screen.getByLabelText(
    "Automatically record current certificate",
  );

  expect(autoPin).toBeEnabled();

  await user.type(
    screen.getByLabelText("Certificate"),
    "-----BEGIN CERTIFICATE-----",
  );

  expect(autoPin).toBeDisabled();
});
