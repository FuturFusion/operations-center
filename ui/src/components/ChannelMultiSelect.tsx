import { FC, KeyboardEvent } from "react";
import { Form } from "react-bootstrap";
import { useChannels } from "context/useChannels";

interface Props {
  value: string[];
  onChange: (value: string[]) => void;
  onKeyDown?: (e: KeyboardEvent<HTMLSelectElement>) => void;
}

const ChannelMultiSelect: FC<Props> = ({ value, onChange, onKeyDown }) => {
  const { data: channels } = useChannels();

  return (
    <Form.Select
      multiple
      value={value}
      onChange={(e) => {
        const selected = Array.from(
          e.target.selectedOptions,
          (option) => option.value,
        );
        onChange(selected);
      }}
      onKeyDown={onKeyDown}
    >
      {channels?.map((channel) => (
        <option key={channel.name} value={channel.name}>
          {channel.name}
        </option>
      ))}
    </Form.Select>
  );
};

export default ChannelMultiSelect;
