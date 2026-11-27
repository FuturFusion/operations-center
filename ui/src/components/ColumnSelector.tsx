import { FC } from "react";
import { Button, Dropdown, Form } from "react-bootstrap";
import { MdViewColumn } from "react-icons/md";
import type { ColumnOverrides, DataTableColumn } from "util/dataTable";

interface Props {
  columns: DataTableColumn[];
  visible: string[];
  autoHidden: string[];
  overrides: ColumnOverrides;
  onChange: (overrides: ColumnOverrides) => void;
}

const ColumnSelector: FC<Props> = ({
  columns,
  visible,
  autoHidden,
  overrides,
  onChange,
}) => {
  const toggle = (name: string) => {
    onChange({ ...overrides, [name]: !visible.includes(name) });
  };

  return (
    <Dropdown autoClose="outside" align="end">
      <Dropdown.Toggle
        size="sm"
        variant="outline-secondary"
        aria-label="Columns"
        title="Columns"
      >
        <MdViewColumn size={18} />
      </Dropdown.Toggle>
      <Dropdown.Menu className="px-3 py-2 column-selector">
        {columns.map((column) => (
          <Form.Check
            key={column.name}
            id={`column-${column.name}`}
            type="checkbox"
            label={
              <>
                {column.name}
                {autoHidden.includes(column.name) && (
                  <span className="text-muted small"> (doesn't fit)</span>
                )}
              </>
            }
            checked={visible.includes(column.name)}
            disabled={!column.optional}
            title={column.optional ? undefined : "Always shown"}
            onChange={() => toggle(column.name)}
          />
        ))}
        <Dropdown.Divider />
        <Button
          variant="link"
          size="sm"
          className="p-0"
          disabled={Object.keys(overrides).length == 0}
          onClick={() => onChange({})}
        >
          Reset to fit the screen
        </Button>
      </Dropdown.Menu>
    </Dropdown>
  );
};

export default ColumnSelector;
