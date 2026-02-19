import { useState } from "react";
import Button from "react-bootstrap/Button";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router";
import { cleanupUpdates, fetchUpdates, refreshUpdates } from "api/update";
import DataTable from "components/DataTable";
import ModalWindow from "components/ModalWindow";
import ImportUpdatesBtn from "components/ImportUpdatesBtn";
import { useNotification } from "context/notificationContext";
import { formatDate } from "util/date";

const Update = () => {
  const [showCleanupModal, setShowCleanupModal] = useState(false);
  const { notify } = useNotification();
  const queryClient = useQueryClient();

  const {
    data: updates = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["updates"],
    queryFn: fetchUpdates,
  });

  const handleRefresh = () => {
    refreshUpdates()
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Updates refresh triggered successfully`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during refreshing updates: ${e}`);
      });
  };

  const handleCleanup = () => {
    cleanupUpdates()
      .then((response) => {
        if (response.error_code == 0) {
          queryClient.invalidateQueries({ queryKey: ["updates"] });
          notify.success(`Updates cleanup performed successfully`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during updates cleanup: ${e}`);
      });
    setShowCleanupModal(false);
  };

  if (isLoading) {
    return <div>Loading updates...</div>;
  }

  if (error) {
    return <div>Error while loading updates: {error.message}</div>;
  }

  const headers = [
    "Version",
    "Published at",
    "Severity",
    "Upstream channels",
    "Downstream channels",
    "Status",
  ];
  const rows = updates.map((item) => {
    return [
      {
        content: (
          <Link
            to={`/ui/provisioning/updates/${item.uuid}`}
            className="data-table-link"
          >
            {item.version}
          </Link>
        ),
        sortKey: item.version,
      },
      {
        content: formatDate(item.published_at),
        sortKey: item.published_at,
      },
      {
        content: item.severity,
        sortKey: item.severity,
      },
      {
        content: (item.upstream_channels ?? []).join(", "),
      },
      {
        content: (item.channels ?? []).join(", "),
      },
      {
        content: item.update_status,
        sortKey: item.update_status,
      },
    ];
  });

  return (
    <>
      <div className="d-flex flex-column">
        <div className="mx-2 mx-md-4">
          <div className="row">
            <div className="col-12">
              <Button
                variant="danger"
                className="float-end mx-2"
                onClick={() => setShowCleanupModal(true)}
              >
                Cleanup
              </Button>
              <Button
                variant="success"
                className="float-end mx-2"
                onClick={handleRefresh}
              >
                Refresh
              </Button>
              <ImportUpdatesBtn />
            </div>
          </div>
        </div>
        <div className="scroll-container flex-grow-1">
          <DataTable headers={headers} rows={rows} />
        </div>
      </div>
      <ModalWindow
        show={showCleanupModal}
        handleClose={() => setShowCleanupModal(false)}
        title="Cleanup updates?"
        footer={
          <>
            <Button variant="danger" onClick={handleCleanup}>
              Cleanup
            </Button>
          </>
        }
      >
        <p>
          Are you sure you want to cleanup updates?
          <br />
          This action cannot be undone.
        </p>
      </ModalWindow>
    </>
  );
};

export default Update;
