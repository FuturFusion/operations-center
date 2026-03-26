import { FC } from "react";
import { Form } from "react-bootstrap";
import { RestoreModeValues } from "util/cluster";

interface Props {
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  label?: string;
  formClasses?: string;
}

const RestoreModeSelect: FC<Props> = ({
  value,
  onChange,
  disabled,
  label,
  formClasses,
}) => {
  return (
    <Form.Group className={formClasses}>
      <Form.Label>{label ?? "Restore Mode"}</Form.Label>
      <Form.Select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
      >
        <option key="" value="">
          {RestoreModeValues[""]}
        </option>
        <option key="" value="skip">
          {RestoreModeValues["skip"]}
        </option>
      </Form.Select>
    </Form.Group>
  );
};

export default RestoreModeSelect;
