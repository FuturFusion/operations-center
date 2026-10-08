import Button from "react-bootstrap/Button";
import Toast from "react-bootstrap/Toast";
import ToastContainer from "react-bootstrap/ToastContainer";
import { MdOutlineClose } from "react-icons/md";
import { useNotification } from "context/notificationContext";
import { notificationVariant } from "util/notification";

const maxVisible = 5;

const Notification = () => {
  const { notifications, dismiss, dismissAll } = useNotification();

  const pending = notifications.filter((n) => n.visible);
  const visible = pending.slice(0, maxVisible);

  return (
    <>
      {visible.length > 0 && (
        <ToastContainer className="p-3" style={{ zIndex: 1 }}>
          {pending.length > 1 && (
            <Button
              variant="secondary"
              size="sm"
              className="mb-2"
              onClick={dismissAll}
            >
              Dismiss all ({pending.length})
            </Button>
          )}
          {visible.map((n) => (
            <Toast key={n.id} bg={notificationVariant[n.type]}>
              <Toast.Body
                className={n.type == "warning" ? "text-dark" : "text-white"}
              >
                <div className="container">
                  <div>
                    <p className="float-end">
                      <MdOutlineClose
                        title="Dismiss"
                        style={{ cursor: "pointer" }}
                        onClick={() => dismiss(n.id)}
                      />
                    </p>
                  </div>
                  <div className="word-wrap">{n.message}</div>
                </div>
              </Toast.Body>
            </Toast>
          ))}
        </ToastContainer>
      )}
    </>
  );
};

export default Notification;
