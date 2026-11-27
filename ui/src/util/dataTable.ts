export interface DataTableColumn {
  name: string;
  // Optional columns can be hidden by the user and give way when space runs out.
  optional?: boolean;
  // Among optional columns, the higher the priority, the longer the column stays.
  priority?: number;
  // Approximate width in pixels, including padding, used to decide what fits.
  width?: number;
}

export type DataTableHeader = string | DataTableColumn;

// Columns the user explicitly showed (true) or hid (false), by name.
export type ColumnOverrides = Record<string, boolean>;

export const defaultColumnWidth = 150;

export const toColumn = (header: DataTableHeader): DataTableColumn =>
  typeof header === "string" ? { name: header } : header;

const columnWidth = (column: DataTableColumn) =>
  column.width ?? defaultColumnWidth;

// autoHiddenColumns returns the optional columns without a user choice, which don't fit into the available width.
export const autoHiddenColumns = (
  columns: DataTableColumn[],
  available: number,
  overrides: ColumnOverrides,
): string[] => {
  // Not laid out yet, show everything.
  if (available <= 0) {
    return [];
  }

  let used = 0;
  const candidates: DataTableColumn[] = [];
  for (const column of columns) {
    const override = overrides[column.name];
    if (!column.optional || override === true) {
      used += columnWidth(column);
    } else if (override === undefined) {
      candidates.push(column);
    }
  }

  // Stable sort keeps the table order among equal priorities.
  candidates.sort((a, b) => (b.priority ?? 0) - (a.priority ?? 0));

  // Once a column doesn't fit, all with lower priority are hidden too, so priorities stay predictable.
  const hidden: string[] = [];
  for (const column of candidates) {
    if (hidden.length == 0 && used + columnWidth(column) <= available) {
      used += columnWidth(column);
      continue;
    }

    hidden.push(column.name);
  }

  return hidden;
};

// visibleColumns returns the indexes of the columns to show.
export const visibleColumns = (
  columns: DataTableColumn[],
  available: number,
  overrides: ColumnOverrides,
): number[] => {
  const autoHidden = autoHiddenColumns(columns, available, overrides);

  return columns.flatMap((column, index) => {
    if (!column.optional) {
      return [index];
    }

    const override = overrides[column.name];
    if (override !== undefined) {
      return override ? [index] : [];
    }

    return autoHidden.includes(column.name) ? [] : [index];
  });
};

const storageKey = (id: string) => `data-table-columns:${id}`;

export const loadColumnOverrides = (id?: string): ColumnOverrides => {
  if (!id) {
    return {};
  }

  try {
    const stored = localStorage.getItem(storageKey(id));
    return stored ? JSON.parse(stored) : {};
  } catch {
    return {};
  }
};

export const saveColumnOverrides = (
  id: string | undefined,
  overrides: ColumnOverrides,
) => {
  if (!id) {
    return;
  }

  if (Object.keys(overrides).length == 0) {
    localStorage.removeItem(storageKey(id));
    return;
  }

  localStorage.setItem(storageKey(id), JSON.stringify(overrides));
};
