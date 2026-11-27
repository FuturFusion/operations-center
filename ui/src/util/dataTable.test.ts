import { expect, test } from "vitest";
import { DataTableColumn, visibleColumns } from "util/dataTable";

// Required columns take 200, the optional ones 100 each.
const columns: DataTableColumn[] = [
  { name: "Name", width: 100 },
  { name: "Low", optional: true, priority: 1, width: 100 },
  { name: "High", optional: true, priority: 3, width: 100 },
  { name: "Mid", optional: true, priority: 2, width: 100 },
  { name: "Actions", width: 100 },
];

const names = (indexes: number[]) => indexes.map((i) => columns[i].name);

test("hides optional columns by priority as the width shrinks", () => {
  expect(names(visibleColumns(columns, 500, {}))).toEqual([
    "Name",
    "Low",
    "High",
    "Mid",
    "Actions",
  ]);
  expect(names(visibleColumns(columns, 450, {}))).toEqual([
    "Name",
    "High",
    "Mid",
    "Actions",
  ]);
  expect(names(visibleColumns(columns, 250, {}))).toEqual(["Name", "Actions"]);

  // Required columns stay even when they don't fit, unmeasured shows all.
  expect(names(visibleColumns(columns, 50, {}))).toHaveLength(2);
  expect(names(visibleColumns(columns, 0, {}))).toHaveLength(5);
});

test("user choices win and the rest fits around them", () => {
  // Hiding High makes room for Low, showing Low pushes out Mid.
  expect(names(visibleColumns(columns, 400, { High: false }))).toEqual([
    "Name",
    "Low",
    "Mid",
    "Actions",
  ]);
  expect(names(visibleColumns(columns, 400, { Low: true }))).toEqual([
    "Name",
    "Low",
    "High",
    "Actions",
  ]);

  // A required column can't be hidden.
  expect(names(visibleColumns(columns, 500, { Name: false }))).toContain(
    "Name",
  );
});
