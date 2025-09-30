import { FC } from "react";
import { Form } from "react-bootstrap";

interface Props {
  value: string;
  onChange: (value: string) => void;
}

const ImageTypeSelect: FC<Props> = ({ value, onChange }) => {
  return (
    <Form.Group className="mb-4" controlId="type">
      <Form.Label>Image type</Form.Label>
      <Form.Check
        type="radio"
        label="ISO (for use with virtual CD-ROM drives)"
        name="type"
        value="iso"
        checked={value == "iso"}
        onChange={() => onChange("iso")}
      />
      <Form.Check
        type="radio"
        label="USB (for use with virtual or physical USB sticks)"
        name="type"
        value="raw"
        checked={value == "raw"}
        onChange={() => onChange("raw")}
      />
    </Form.Group>
  );
};

export default ImageTypeSelect;
