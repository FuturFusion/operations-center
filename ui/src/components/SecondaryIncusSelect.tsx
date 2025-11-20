import { FC } from "react";
import { Form } from "react-bootstrap";
import { secondaryIncusAppOptions } from "util/util";

interface Props {
  value: string[];
  onChange: (value: string, checked: boolean) => void;
}

const SecondaryIncusSelect: FC<Props> = ({ value, onChange }) => {
  return (
    <Form.Group className="mb-4" controlId="type">
      <Form.Label>Secondary application</Form.Label>
      {Object.entries(secondaryIncusAppOptions).map(([key, label]) => (
        <Form.Check
          type="checkbox"
          label={label}
          name={key}
          checked={value.includes(key)}
          onChange={(e) => onChange(key, e.target.checked)}
        />
      ))}
    </Form.Group>
  );
};

export default SecondaryIncusSelect;
