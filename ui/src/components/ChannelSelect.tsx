import { FC } from "react";
import { Form } from "react-bootstrap";
import { useChannels } from "context/useChannels";

interface Props {
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  label?: string;
  formClasses: string;
}

const ChannelSelect: FC<Props> = ({
  value,
  onChange,
  disabled,
  label,
  formClasses,
}) => {
  const { data: channels } = useChannels();

  return (
    <Form.Group className={formClasses}>
      <Form.Label>{label ?? "Channel"}</Form.Label>
      <Form.Select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
      >
        <option key="" value=""></option>
        {channels?.map((channel) => (
          <option key={channel.name} value={channel.name}>
            {channel.name}
          </option>
        ))}
      </Form.Select>
    </Form.Group>
  );
};

export default ChannelSelect;
