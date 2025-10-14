import { FC, ReactNode, useState, useRef } from "react";
import { Col, Form, Row, Table } from "react-bootstrap";
import { MdArrowDropDown, MdArrowDropUp } from "react-icons/md";

export interface DataTableRow {
  content: ReactNode | string;
  sortKey?: string | number;
  class?: string;
}

interface Props {
  headers: string[];
  rows: DataTableRow[][];
}

const DataTable: FC<Props> = ({ headers, rows }) => {
  const [currentPage, setCurrentPage] = useState(1);
  const [itemsPerPage, setItemsPerPage] = useState(20);
  const [sortProps, setSortProps] = useState({ order: "", column: "" });
  const headersMap = useRef(
    Object.fromEntries(headers.map((item, index) => [item, index])),
  );

  const totalPages = Math.ceil(rows.length / itemsPerPage);

  const indexOfLastItem = currentPage * itemsPerPage;
  const indexOfFirstItem = indexOfLastItem - itemsPerPage;

  const isSortable = (column: string) => {
    const itemIndex = headersMap.current[column];

    if (
      column !== "" &&
      rows.length > 0 &&
      rows[0][itemIndex].sortKey !== undefined
    ) {
      return true;
    }
    return false;
  };

  if (isSortable(sortProps.column)) {
    rows.sort((a, b) => {
      const itemIndex = headersMap.current[sortProps.column];

      const aSortKey = a[itemIndex].sortKey;
      const bSortKey = b[itemIndex].sortKey;

      // If the sortKey property is missing on any item, then do not perform sorting.
      if (aSortKey === undefined || bSortKey === undefined) {
        return 0;
      }

      console.log(aSortKey, bSortKey, typeof aSortKey);
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

  const paginatedData = rows.slice(indexOfFirstItem, indexOfLastItem);

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
    const headerRow = headers.map((item, index) => {
      const sortEnabled = isSortable(item);
      return (
        <th
          key={index}
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
      const row = rowItem.map((item, index) => {
        return (
          <td className={item.class} key={index}>
            {item.content}
          </td>
        );
      });

      return <tr key={rowIndex}>{row}</tr>;
    });

    return <>{dataRows}</>;
  };

  return (
    <div className="mx-2 mx-md-4 mt-4">
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
      </Row>
      <Table className="data-table" hover>
        <thead>{generateHeaders()}</thead>
        <tbody>{generateRows()}</tbody>
      </Table>
    </div>
  );
};

export default DataTable;
