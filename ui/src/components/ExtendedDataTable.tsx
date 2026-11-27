import { FC } from "react";
import DataTable, { DataTableProps } from "components/DataTable";

interface Props extends DataTableProps {
  isLoading: boolean;
  error: Error | null;
}

const ExtendedDataTable: FC<Props> = ({ isLoading, error, ...props }) => {
  if (isLoading) {
    return <div>Loading data...</div>;
  }

  if (error) {
    return (
      <div>
        Error while loading data: <pre>{error.message}</pre>
      </div>
    );
  }

  return <DataTable {...props} />;
};

export default ExtendedDataTable;
