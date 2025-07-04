import { useState } from "react";
import { Button } from "react-bootstrap";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import { deleteServer, fetchServer } from "api/server";
import ModalWindow from "components/ModalWindow";
import TabView from "components/TabView";
import { useNotification } from "context/notificationContext";
import ServerOverview from "pages/ServerOverview";
import ServerConfiguration from "pages/ServerConfiguration";

const ServerDetail = () => {
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const queryClient = useQueryClient();
  const { notify } = useNotification();
  const navigate = useNavigate();
  const { name, activeTab } = useParams<{ name: string; activeTab: string }>();

  const {
    data: server = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["servers", name],
    queryFn: () => fetchServer(name || ""),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error || !server) {
    return <div>Error while loading server</div>;
  }

  const handleDelete = () => {
    deleteServer(name || "")
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Server ${name} deleted`);
          queryClient.invalidateQueries({ queryKey: ["servers"] });
          navigate("/ui/provisioning/servers");
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during server deletion: ${e}`);
      });
  };

  const tabs = [
    {
      key: "overview",
      title: "Overview",
      content: <ServerOverview />,
    },
    {
      key: "configuration",
      title: "Configuration",
      content: <ServerConfiguration />,
    },
  ];

  return (
    <div className="d-flex flex-column">
      <div className="scroll-container flex-grow-1 p-3">
        <TabView
          defaultTab="overview"
          activeTab={activeTab}
          tabs={tabs}
          onSelect={(key) =>
            navigate(`/ui/provisioning/servers/${name}/${key}`)
          }
        />
      </div>
      <div className="fixed-footer p-3">
        {(!activeTab || activeTab == "overview") && (
          <div className="d-flex justify-content-end gap-2">
            <Button variant="danger" onClick={() => setShowDeleteModal(true)}>
              Delete
            </Button>
          </div>
        )}
      </div>
      <ModalWindow
        show={showDeleteModal}
        handleClose={() => setShowDeleteModal(false)}
        title="Delete Server?"
        footer={
          <>
            <Button variant="danger" onClick={handleDelete}>
              Delete
            </Button>
          </>
        }
      >
        <p>
          Are you sure you want to delete the server "{name}"?
          <br />
          This action cannot be undone.
        </p>
      </ModalWindow>
    </div>
  );
};

export default ServerDetail;
