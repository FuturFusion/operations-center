import { useState } from "react";
import Button from "react-bootstrap/Button";
import Form from "react-bootstrap/Form";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router";
import { cleanupUpdates, fetchUpdates, refreshUpdates } from "api/update";
import DataTable from "components/DataTable";
import ModalWindow from "components/ModalWindow";
import ImportUpdatesBtn from "components/ImportUpdatesBtn";
import UpdateActions from "components/UpdateActions";
import { useNotification } from "context/notificationContext";
import { formatDate } from "util/date";
import { errorMessage } from "util/response";

const Update = () => {
  const refetchInterval = 10000; // 10 seconds
  const [showCleanupModal, setShowCleanupModal] = useState(false);
  const [forceCleanup, setForceCleanup] = useState(false);
  const { notify } = useNotification();
  const queryClient = useQueryClient();

  const {
    data: updates = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["updates"],
    queryFn: fetchUpdates,
    refetchInterval: refetchInterval,
  });

  const handleRefresh = () => {
    refreshUpdates()
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Updates refresh triggered successfully`);
          return;
        }
        notify.error(errorMessage(response));
      })
      .catch((e) => {
        notify.error(`Error during refreshing updates: ${e}`);
      });
  };

  const handleCleanup = () => {
    cleanupUpdates(forceCleanup)
      .then((response) => {
        if (response.error_code == 0) {
          queryClient.invalidateQueries({ queryKey: ["updates"] });
          notify.success(`Updates cleanup performed successfully`);
          return;
        }
        notify.error(errorMessage(response));
      })
      .catch((e) => {
        notify.error(`Error during updates cleanup: ${e}`);
      });
    closeCleanupModal();
  };

  const closeCleanupModal = () => {
    setShowCleanupModal(false);
    setForceCleanup(false);
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
    "Actions",
  ];
  const rows = updates.map((item) => {
    return {
      cols: [
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
        {
          content: <UpdateActions update={item} />,
        },
      ],
    };
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
        <div className="flex-grow-1">
          <DataTable headers={headers} rows={rows} />
        </div>
      </div>
      <ModalWindow
        show={showCleanupModal}
        handleClose={closeCleanupModal}
        title="Cleanup updates?"
        footer={
          <>
            <Button variant="danger" onClick={handleCleanup}>
              Cleanup
            </Button>
          </>
        }
      >
        <p>Cleanup removes updates and frees up the disk space used by them.</p>
        <p>
          Protected updates are kept. An update is protected if a server runs
          its version, or if it is assigned to a channel other than the default
          channel.
        </p>
        <Form.Check
          type="checkbox"
          id="force-cleanup"
          label="Also remove protected updates"
          checked={forceCleanup}
          onChange={(e) => setForceCleanup(e.target.checked)}
        />
        {forceCleanup && (
          <p className="text-danger mt-2">
            All updates and all channel assignments are removed. Updates in use
            by servers can not be downloaded again if they are no longer
            available upstream. Use this only if the disk is full and the
            default cleanup fails.
          </p>
        )}
        <p className="mt-2">This action cannot be undone.</p>
      </ModalWindow>
    </>
  );
};

export default Update;
