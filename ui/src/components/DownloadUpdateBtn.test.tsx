import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, test, vi } from "vitest";
import { downloadUpdate } from "api/update";
import DownloadUpdateBtn from "components/DownloadUpdateBtn";

vi.mock("api/update", () => ({
  downloadUpdate: vi.fn(() => Promise.resolve({ error_code: 0 })),
}));

const renderBtn = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { staleTime: Infinity } },
  });
  queryClient.setQueryData(
    ["channels"],
    [{ name: "stable" }, { name: "legacy" }],
  );

  render(
    <QueryClientProvider client={queryClient}>
      <DownloadUpdateBtn />
    </QueryClientProvider>,
  );
};

test("download is disabled without a version", () => {
  renderBtn();

  fireEvent.click(screen.getByRole("button", { name: "Download" }));

  const buttons = screen.getAllByRole("button", { name: "Download" });
  expect(buttons[buttons.length - 1]).toBeDisabled();
});

test("downloads the given version into the selected channels", async () => {
  renderBtn();

  fireEvent.click(screen.getByRole("button", { name: "Download" }));
  fireEvent.change(screen.getByLabelText("Version"), {
    target: { value: " 202610272234 " },
  });

  const legacy = screen.getByRole("option", { name: "legacy" });
  (legacy as HTMLOptionElement).selected = true;
  fireEvent.change(screen.getByLabelText("Channels"));

  const buttons = screen.getAllByRole("button", { name: "Download" });
  fireEvent.click(buttons[buttons.length - 1]);

  await waitFor(() =>
    expect(downloadUpdate).toHaveBeenCalledWith("202610272234", ["legacy"]),
  );
});
