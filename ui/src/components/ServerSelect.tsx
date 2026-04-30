import { FC, KeyboardEvent } from "react";
import { Form } from "react-bootstrap";
import { Server } from "types/server";
import { handleCtrlA } from "util/util";

interface Props {
  servers: Server[];
  selected: string[];
  disabled: boolean;
  onChange: (values: string[]) => void;
}

const ServerSelect: FC<Props> = ({ servers, selected, disabled, onChange }) => {
  const handleServersKeyDown = (e: KeyboardEvent<HTMLSelectElement>) => {
    e.preventDefault();
    onChange(servers?.map((s) => s.name) ?? []);
  };

  return (
    <Form.Select
      multiple
      value={selected}
      onChange={(e) => {
        const newValues = Array.from(
          e.target.selectedOptions,
          (option) => option.value,
        );
        onChange(newValues);
      }}
      onKeyDown={handleCtrlA(handleServersKeyDown)}
      disabled={disabled}
    >
      {servers?.map((server) => (
        <option key={server.name} value={server.name}>
          {server.name}
        </option>
      ))}
    </Form.Select>
  );
};

export default ServerSelect;
