import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import TokenForm from "components/TokenForm";
import { Token } from "types/token";

vi.mock("context/useChannels", () => ({
  useChannels: () => ({
    data: [
      { name: "stable", description: "" },
      { name: "testing", description: "" },
    ],
  }),
}));

const token: Token = {
  uuid: "b32d0079-c48b-4957-b1cb-bef54125c861",
  description: "token",
  channel: "stable",
  expire_at: "2026-12-01T00:00:00Z",
  uses_remaining: 5,
};

test("shows the channel of the token", () => {
  render(<TokenForm token={token} onSubmit={vi.fn()} />);

  expect(screen.getByRole("combobox")).toHaveValue("stable");
});

test("submits the changed channel", async () => {
  const handleSubmit = vi.fn();
  render(<TokenForm token={token} onSubmit={handleSubmit} />);

  fireEvent.change(screen.getByRole("combobox"), {
    target: { value: "testing" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Submit" }));

  await waitFor(() => {
    expect(handleSubmit).toHaveBeenCalledWith({
      description: "token",
      channel: "testing",
      expire_at: "2026-12-01T00:00:00Z",
      uses_remaining: 5,
    });
  });
});
