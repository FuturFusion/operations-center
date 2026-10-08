import { useState } from "react";
import { Badge, Button, ListGroup, Offcanvas } from "react-bootstrap";
import { MdNotificationsNone } from "react-icons/md";
import { NavItemLink } from "components/NavItemLink";
import { useNotification } from "context/notificationContext";
import { formatDateTime } from "util/date";
import { notificationVariant } from "util/notification";

const NotificationHistory = () => {
  const { notifications, clear } = useNotification();
  const [show, setShow] = useState(false);

  const problemCount = notifications.filter(
    (n) => n.visible && (n.type == "error" || n.type == "warning"),
  ).length;

  return (
    <>
      <NavItemLink as="button" onClick={() => setShow(true)}>
        <MdNotificationsNone /> Notifications{" "}
        {problemCount > 0 && (
          <Badge bg="danger" pill>
            {problemCount}
          </Badge>
        )}
      </NavItemLink>
      <Offcanvas show={show} onHide={() => setShow(false)} placement="end">
        <Offcanvas.Header closeButton>
          <Offcanvas.Title>Notifications</Offcanvas.Title>
        </Offcanvas.Header>
        <Offcanvas.Body>
          {notifications.length == 0 && <p>No notifications</p>}
          {notifications.length > 0 && (
            <>
              <Button
                variant="outline-secondary"
                size="sm"
                className="mb-3"
                onClick={clear}
              >
                Clear all
              </Button>
              <ListGroup variant="flush">
                {notifications.map((n) => (
                  <ListGroup.Item key={n.id}>
                    <Badge bg={notificationVariant[n.type]}>{n.type}</Badge>{" "}
                    <small style={{ color: "var(--bs-secondary-color)" }}>
                      {formatDateTime(n.timestamp)}
                    </small>
                    <div className="word-wrap">{n.message}</div>
                  </ListGroup.Item>
                ))}
              </ListGroup>
            </>
          )}
        </Offcanvas.Body>
      </Offcanvas>
    </>
  );
};

export default NotificationHistory;
