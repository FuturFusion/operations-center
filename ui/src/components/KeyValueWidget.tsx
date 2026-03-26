import { FC, useEffect, useState } from "react";
import { Button, Form, Table } from "react-bootstrap";
import { BsPlus, BsTrash } from "react-icons/bs";

type KeyValueMap = Record<string, string>;

interface Props {
  value: KeyValueMap;
  onChange: (value: KeyValueMap) => void;
}

const KeyValueWidget: FC<Props> = ({ value, onChange }) => {
  const [entries, setEntries] = useState<KeyValueMap>(value || {});
  const [newKey, setNewKey] = useState("");
  const [newValue, setNewValue] = useState("");

  const handleAdd = () => {
    if (!newKey || newKey in entries) return;
    const newValues = { ...entries, [newKey]: newValue };
    setEntries(newValues);
    onChange(newValues);
    setNewKey("");
    setNewValue("");
  };

  useEffect(() => {
    setEntries(value || {});
  }, [value]);

  const handleDelete = (key: string) => {
    const { [key]: _, ...rest } = entries;

    setEntries(rest);
    onChange(rest);
  };

  const handleEdit = (key: string, value: string) => {
    const newValues = { ...entries, [key]: value };
    setEntries(newValues);
    onChange(newValues);
  };

  return (
    <div>
      <Table>
        <tbody>
          {Object.entries(entries).map(([key, value]) => (
            <tr key={key}>
              <td className="key-item">{key}</td>
              <td>
                <Form.Control
                  type="text"
                  size="sm"
                  value={value}
                  onChange={(e) => handleEdit(key, e.target.value)}
                />
              </td>
              <td>
                <Button
                  title="Delete"
                  size="sm"
                  variant="outline-secondary"
                  className="bg-white border no-hover"
                  onClick={() => handleDelete(key)}
                >
                  <BsTrash />
                </Button>
              </td>
            </tr>
          ))}
          <tr>
            <td>
              <Form.Control
                type="text"
                size="sm"
                placeholder="New key"
                value={newKey}
                onChange={(e) => setNewKey(e.target.value)}
              />
            </td>
            <td>
              <Form.Control
                type="text"
                size="sm"
                placeholder="New value"
                value={newValue}
                onChange={(e) => setNewValue(e.target.value)}
              />
            </td>
            <td>
              <Button
                title="Add"
                size="sm"
                variant="outline-secondary"
                className="bg-white border no-hover"
                onClick={handleAdd}
                disabled={!newKey || newKey in entries}
              >
                <BsPlus />
              </Button>
            </td>
          </tr>
        </tbody>
      </Table>
    </div>
  );
};

export default KeyValueWidget;
