import {
  FC,
  ReactNode,
  RefObject,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { Col, Form, Row, Table } from "react-bootstrap";
import { MdArrowDropDown, MdArrowDropUp } from "react-icons/md";
import ColumnSelector from "components/ColumnSelector";
import {
  autoHiddenColumns,
  ColumnOverrides,
  DataTableHeader,
  loadColumnOverrides,
  saveColumnOverrides,
  toColumn,
  visibleColumns,
} from "util/dataTable";

interface DataTableCol {
  content: ReactNode | string;
  sortKey?: string | number;
  class?: string;
}

export interface DataTableRow {
  additional_data?: ReactNode | string;
  cols: DataTableCol[];
}

export interface DataTableProps {
  headers: DataTableHeader[];
  rows: DataTableRow[];
  // Remembers the user's column choices across visits under this id.
  id?: string;
}

// useElementWidth tracks the inner width of the element.
const useElementWidth = (ref: RefObject<HTMLElement | null>) => {
  const [width, setWidth] = useState(0);

  useLayoutEffect(() => {
    const element = ref.current;
    if (!element) {
      return;
    }

    const update = () => setWidth(element.clientWidth);
    update();

    if (typeof ResizeObserver === "undefined") {
      window.addEventListener("resize", update);
      return () => window.removeEventListener("resize", update);
    }

    const observer = new ResizeObserver(update);
    observer.observe(element);
    return () => observer.disconnect();
  }, [ref]);

  return width;
};

const DataTable: FC<DataTableProps> = ({ headers, rows, id }) => {
  const [currentPage, setCurrentPage] = useState(1);
  const [itemsPerPage, setItemsPerPage] = useState(20);
  const [sortProps, setSortProps] = useState({ order: "", column: "" });
  const [overrides, setOverrides] = useState<ColumnOverrides>(() =>
    loadColumnOverrides(id),
  );
  const containerRef = useRef<HTMLDivElement>(null);
  const width = useElementWidth(containerRef);

  const columns = headers.map(toColumn);
  const shown = visibleColumns(columns, width, overrides);
  const autoHidden = autoHiddenColumns(columns, width, overrides);
  const hasOptional = columns.some((column) => column.optional);

  const headersMap = Object.fromEntries(
    columns.map((item, index) => [item.name, index]),
  );

  const changeOverrides = (value: ColumnOverrides) => {
    setOverrides(value);
    saveColumnOverrides(id, value);
  };

  const totalPages = Math.ceil(rows.length / itemsPerPage);

  const indexOfLastItem = currentPage * itemsPerPage;
  const indexOfFirstItem = indexOfLastItem - itemsPerPage;

  const isSortable = (column: string) => {
    const itemIndex = headersMap[column];

    if (
      column !== "" &&
      rows.length > 0 &&
      rows[0].cols[itemIndex].sortKey !== undefined
    ) {
      return true;
    }
    return false;
  };

  // Sort a copy, the rows are owned by the caller.
  const sortedRows = [...rows];
  if (isSortable(sortProps.column)) {
    sortedRows.sort((a, b) => {
      const itemIndex = headersMap[sortProps.column];

      const aSortKey = a.cols[itemIndex].sortKey;
      const bSortKey = b.cols[itemIndex].sortKey;

      // If the sortKey property is missing on any item, then do not perform sorting.
      if (aSortKey === undefined || bSortKey === undefined) {
        return 0;
      }

      if (sortProps.order === "asc") {
        if (typeof aSortKey === "number" && typeof bSortKey == "number") {
          return aSortKey - bSortKey;
        } else if (
          typeof aSortKey === "string" &&
          typeof bSortKey == "string"
        ) {
          return aSortKey.localeCompare(bSortKey);
        }
        // Put numbers before strings.
        return typeof aSortKey === "number" ? -1 : 1;
      } else {
        if (typeof aSortKey === "number" && typeof bSortKey == "number") {
          return bSortKey - aSortKey;
        } else if (
          typeof aSortKey === "string" &&
          typeof bSortKey == "string"
        ) {
          return bSortKey.localeCompare(aSortKey);
        }
        return typeof aSortKey === "number" ? 1 : -1;
      }
    });
  }

  const paginatedData = sortedRows.slice(indexOfFirstItem, indexOfLastItem);

  // After changing the number of items per page,
  // it may turn out that currentPage > totalPages. So, set currentPage to 1.
  if (totalPages > 0 && currentPage > totalPages) {
    setCurrentPage(1);
  }

  const handleHeaderClick = (column: string) => {
    // When columns are changed, perform ascending sorting.
    if (column !== sortProps.column) {
      setSortProps({ order: "asc", column: column });
      return;
    }

    if (sortProps.order === "asc") {
      setSortProps({ order: "desc", column: sortProps.column });
    } else {
      setSortProps({ order: "asc", column: sortProps.column });
    }
  };

  const handlePageChange = (page: number) => {
    if (page > totalPages) {
      page = totalPages;
    } else if (page < 1) {
      page = 1;
    }

    setCurrentPage(page);
  };

  const generateHeaders = () => {
    const headerRow = shown.map((index) => {
      const item = columns[index].name;
      const sortEnabled = isSortable(item);
      return (
        <th
          key={item}
          style={{ cursor: sortEnabled ? "pointer" : "default" }}
          onClick={sortEnabled ? () => handleHeaderClick(item) : undefined}
        >
          {item}
          {item === sortProps.column && sortProps.order == "asc" && (
            <MdArrowDropUp size={20} />
          )}
          {item === sortProps.column && sortProps.order == "desc" && (
            <MdArrowDropDown size={20} />
          )}
        </th>
      );
    });

    return <tr>{headerRow}</tr>;
  };

  const generateRows = () => {
    const dataRows = paginatedData.map((rowItem, rowIndex) => {
      const row = shown.map((index) => {
        const item = rowItem.cols[index];
        return (
          <td className={item.class} key={index}>
            {item.content}
          </td>
        );
      });

      return (
        <>
          <tr
            key={rowIndex}
            className={rowItem.additional_data ? "no-bottom-border" : ""}
          >
            {row}
          </tr>
          {rowItem.additional_data && (
            <tr key={rowIndex} className="additional-data-row">
              <td colSpan={shown.length}>{rowItem.additional_data}</td>
            </tr>
          )}
        </>
      );
    });

    return <>{dataRows}</>;
  };

  return (
    <div className="mx-2 mx-md-4 mt-4" ref={containerRef}>
      <Row className="justify-content-end">
        <Col xs="auto">
          <Form.Control
            type="number"
            name="currentPage"
            size="sm"
            className="page-control"
            value={currentPage}
            min={1}
            max={totalPages > 0 ? totalPages : 1}
            onChange={(e) => handlePageChange(Number(e.target.value))}
          />{" "}
          of {totalPages > 0 ? totalPages : 1}
        </Col>
        <Col xs="auto">
          <Form.Select
            size="sm"
            onChange={(e) => setItemsPerPage(Number(e.target.value))}
          >
            <option value={20}>20</option>
            <option value={50}>50</option>
            <option value={100}>100</option>
          </Form.Select>
        </Col>
        {hasOptional && (
          <Col xs="auto">
            <ColumnSelector
              columns={columns}
              visible={shown.map((index) => columns[index].name)}
              autoHidden={autoHidden}
              overrides={overrides}
              onChange={changeOverrides}
            />
          </Col>
        )}
      </Row>
      <Table className="data-table" hover responsive>
        <thead>{generateHeaders()}</thead>
        <tbody>{generateRows()}</tbody>
      </Table>
    </div>
  );
};

export default DataTable;
