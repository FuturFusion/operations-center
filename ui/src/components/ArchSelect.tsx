import { FC } from "react";
import { Form } from "react-bootstrap";

const archValues = ["", "x86_64", "aarch64"];
type Arch = (typeof archValues)[number];

interface Props {
  value: Arch;
  onChange: (value: Arch) => void;
  emptyLabel?: string;
}

const ArchSelect: FC<Props> = ({ value, onChange, emptyLabel = "" }) => {
  return (
    <Form.Group>
      <Form.Label>Architecture</Form.Label>
      <Form.Select
        value={value}
        onChange={(e) => onChange(e.target.value as Arch)}
      >
        {archValues.map((arch) => (
          <option key={arch} value={arch}>
            {arch === "" ? emptyLabel : arch}
          </option>
        ))}
      </Form.Select>
    </Form.Group>
  );
};

export default ArchSelect;
