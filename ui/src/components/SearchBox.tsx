import { FC, useState, KeyboardEvent, ChangeEvent } from "react";
import { Form, InputGroup, Button } from "react-bootstrap";
import { MdClear, MdOutlineSearch } from "react-icons/md";

interface Props {
  value?: string;
  onSearch: (value: string) => void;
}

const SearchBox: FC<Props> = ({ value, onSearch }) => {
  const [input, setInput] = useState(value);

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      onSearch(input || "");
    }
  };

  return (
    <>
      <InputGroup style={{ width: "50vw" }}>
        <Form.Control
          type="text"
          name="search"
          placeholder="Search"
          value={input}
          onChange={(e: ChangeEvent<HTMLInputElement>) =>
            setInput(e.target.value)
          }
          onKeyDown={handleKeyDown}
        />
        {input && (
          <Button
            variant="outline-secondary"
            onClick={() => {
              setInput("");
              onSearch("");
            }}
          >
            <MdClear />
          </Button>
        )}
        <Button
          variant="outline-secondary"
          onClick={() => onSearch(input || "")}
        >
          <MdOutlineSearch />
        </Button>
      </InputGroup>
    </>
  );
};

export default SearchBox;
