import { FC, useEffect, useState } from "react";
import { Button, Form, Table } from "react-bootstrap";
import { BsPlus, BsTrash } from "react-icons/bs";
import { ClusterTemplateVariable } from "types/cluster_template";

type ClusterTemplateVariables = Record<string, ClusterTemplateVariable>;

interface Props {
  value: ClusterTemplateVariables;
  onChange: (value: ClusterTemplateVariables) => void;
}

const ClusterTemplateVariablesWidget: FC<Props> = ({ value, onChange }) => {
  const [entries, setEntries] = useState<ClusterTemplateVariables>(value || {});
  const [newName, setNewName] = useState("");
  const [newValue, setNewValue] = useState({ description: "", default: "" });

  const handleAdd = () => {
    if (!newName || newName in entries) return;
    const newValues = {
      ...entries,
      [newName]: {
        description: newValue.description,
        default: newValue.default,
      },
    };
    setEntries(newValues);
    onChange(newValues);
    setNewName("");
    setNewValue({ description: "", default: "" });
  };

  useEffect(() => {
    setEntries(value || {});
  }, [value]);

  const handleDelete = (key: string) => {
    const { [key]: _, ...rest } = entries;

    setEntries(rest);
    onChange(rest);
  };

  const handleEdit = (key: string, value: ClusterTemplateVariable) => {
    const newValues = {
      ...entries,
      [key]: value,
    };
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
                  value={value.description}
                  onChange={(e) =>
                    handleEdit(key, { ...value, description: e.target.value })
                  }
                />
              </td>
              <td>
                <Form.Control
                  type="text"
                  size="sm"
                  value={value.default}
                  onChange={(e) =>
                    handleEdit(key, { ...value, default: e.target.value })
                  }
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
                placeholder="New variable"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
              />
            </td>
            <td>
              <Form.Control
                type="text"
                size="sm"
                placeholder="Description"
                value={newValue.description}
                onChange={(e) =>
                  setNewValue({ ...newValue, description: e.target.value })
                }
              />
            </td>
            <td>
              <Form.Control
                type="text"
                size="sm"
                placeholder="Default value"
                value={newValue.default}
                onChange={(e) =>
                  setNewValue({ ...newValue, default: e.target.value })
                }
              />
            </td>
            <td>
              <Button
                title="Add"
                size="sm"
                variant="outline-secondary"
                className="bg-white border no-hover"
                onClick={handleAdd}
                disabled={!newName || newName in entries}
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

export default ClusterTemplateVariablesWidget;
