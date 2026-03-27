import { FC, useState } from "react";
import { Card, ListGroup, Badge, Collapse } from "react-bootstrap";
import { BsChevronDown, BsChevronUp } from "react-icons/bs";
import { Changelog } from "types/changelog";

type Props = {
  changelog?: Changelog;
};

const ChangeLogView: FC<Props> = ({ changelog }) => {
  const [openKeys, setOpenKeys] = useState<Record<string, boolean>>({});

  const toggleKey = (key: string) => {
    setOpenKeys((prev) => ({ ...prev, [key]: !prev[key] }));
  };

  const renderSection = (title: string, items?: string[], variant?: string) => {
    if (!items || items.length === 0) return null;

    return (
      <>
        <h6 className="mt-2">
          <Badge bg={variant || "secondary"}>{title}</Badge>
        </h6>
        <ListGroup variant="flush">
          {items.map((item, idx) => (
            <ListGroup.Item key={idx}>{item}</ListGroup.Item>
          ))}
        </ListGroup>
      </>
    );
  };

  return (
    <div>
      {Object.entries(changelog?.components ?? {}).map(([key, entry]) => (
        <Card className="mb-1" key={key} style={{ fontSize: "0.8rem" }}>
          <Card.Header
            className="d-flex justify-content-between align-items-center py-1"
            style={{ cursor: "pointer" }}
            onClick={() => toggleKey(key)}
          >
            <strong>{key}</strong>
            {openKeys[key] ? <BsChevronUp /> : <BsChevronDown />}
          </Card.Header>

          <Collapse in={!!openKeys[key]}>
            <Card.Body className="py-2 px-2">
              {renderSection("Added", entry.added, "success")}
              {renderSection("Updated", entry.updated, "warning")}
              {renderSection("Removed", entry.removed, "danger")}
            </Card.Body>
          </Collapse>
        </Card>
      ))}
    </div>
  );
};

export default ChangeLogView;
