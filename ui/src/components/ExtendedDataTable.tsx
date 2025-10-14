import { FC } from "react";
import DataTable from "components/DataTable";
import { DataTableRow } from "components/DataTable";

interface Props {
  headers: string[];
  rows: DataTableRow[][];
  isLoading: boolean;
  error: Error | null;
}

const ExtendedDataTable: FC<Props> = ({ headers, rows, isLoading, error }) => {
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

  return <DataTable headers={headers} rows={rows} />;
};

export default ExtendedDataTable;
