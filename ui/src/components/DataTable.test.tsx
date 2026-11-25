import { screen, within } from "@testing-library/react";
import { expect, test } from "vitest";
import DataTable, { DataTableRow } from "components/DataTable";
import { renderWithProviders } from "test/render";

const headers = ["Name", "Size", "Actions"];

const row = (name: string, size: number | string): DataTableRow => ({
  cols: [
    { content: name, sortKey: name },
    { content: String(size), sortKey: size },
    { content: "edit" },
  ],
});

// Names of the rows in the order shown.
const shownNames = () =>
  screen
    .getAllByRole("row")
    .slice(1)
    .map((r) => within(r).getAllByRole("cell")[0].textContent);

test("sorts by the clicked column, toggling the order", async () => {
  const { user } = renderWithProviders(
    <DataTable
      headers={headers}
      rows={[row("b", 10), row("c", 2), row("a", 30)]}
    />,
  );

  expect(shownNames()).toEqual(["b", "c", "a"]);

  await user.click(screen.getByText("Name"));
  expect(shownNames()).toEqual(["a", "b", "c"]);

  await user.click(screen.getByText("Name"));
  expect(shownNames()).toEqual(["c", "b", "a"]);

  // Another column starts ascending, comparing numbers numerically.
  await user.click(screen.getByText("Size"));
  expect(shownNames()).toEqual(["c", "b", "a"]);

  await user.click(screen.getByText("Size"));
  expect(shownNames()).toEqual(["a", "b", "c"]);
});

test("sorts numbers before strings", async () => {
  const { user } = renderWithProviders(
    <DataTable
      headers={headers}
      rows={[row("x", "unknown"), row("y", 5), row("z", "n/a"), row("w", 1)]}
    />,
  );

  await user.click(screen.getByText("Size"));
  expect(shownNames()).toEqual(["w", "y", "z", "x"]);

  await user.click(screen.getByText("Size"));
  expect(shownNames()).toEqual(["x", "z", "y", "w"]);
});

test("does not sort by a column without sort key", async () => {
  const { user } = renderWithProviders(
    <DataTable headers={headers} rows={[row("b", 1), row("a", 2)]} />,
  );

  await user.click(screen.getByText("Actions"));
  expect(shownNames()).toEqual(["b", "a"]);
});

test("paginates the rows", async () => {
  const rows = Array.from({ length: 45 }, (_, i) =>
    row(`server${String(i).padStart(2, "0")}`, i),
  );
  const { user } = renderWithProviders(
    <DataTable headers={headers} rows={rows} />,
  );

  const page = screen.getByRole("spinbutton");

  expect(screen.getByText(/of 3/)).toBeInTheDocument();
  expect(shownNames()).toHaveLength(20);
  expect(shownNames()[0]).toBe("server00");

  await user.type(page, "3", {
    initialSelectionStart: 0,
    initialSelectionEnd: 1,
  });
  expect(shownNames()).toEqual([
    "server40",
    "server41",
    "server42",
    "server43",
    "server44",
  ]);

  // Beyond the last page shows the last page.
  await user.type(page, "9", {
    initialSelectionStart: 0,
    initialSelectionEnd: 1,
  });
  expect(page).toHaveValue(3);

  // Fewer pages than the current one go back to the first page.
  await user.selectOptions(screen.getByRole("combobox"), "50");
  expect(screen.getByText(/of 1/)).toBeInTheDocument();
  expect(page).toHaveValue(1);
  expect(shownNames()).toHaveLength(45);
});
