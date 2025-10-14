import { FC } from "react";
import { useSearchParams } from "react-router";
import SearchBox from "components/SearchBox";

const InventorySearchBox: FC = () => {
  const [searchParams, setSearchParams] = useSearchParams();

  const handleSearch = (input: string) => {
    const trimmed = input.trim();
    setSearchParams({ filter: trimmed });
  };

  return (
    <>
      <SearchBox
        value={searchParams.get("filter") || ""}
        onSearch={handleSearch}
      />
    </>
  );
};

export default InventorySearchBox;
