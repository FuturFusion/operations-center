import { useState } from "react";
import { Button, Form } from "react-bootstrap";
import { useNavigate, useParams } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { deleteCluster, fetchCluster } from "api/cluster";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import TabView from "components/TabView";
import { useNotification } from "context/notificationContext";
import ClusterArtifacts from "pages/ClusterArtifacts";
import ClusterConfiguration from "pages/ClusterConfiguration";
import ClusterOverview from "pages/ClusterOverview";

type DeleteMode = "normal" | "force" | "factory-reset";

const ClusterDetail = () => {
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [deleteInProgress, setDeleteInProgress] = useState(false);
  const [deletionMode, setDeletionMode] = useState<DeleteMode>("normal");
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { notify } = useNotification();
  const { name, activeTab } = useParams<{ name: string; activeTab: string }>();

  const {
    data: cluster = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["clusters", name],
    queryFn: () => fetchCluster(name || ""),
  });

  const handleDelete = () => {
    setDeleteInProgress(true);
    deleteCluster(name || "", deletionMode)
      .then((response) => {
        setDeleteInProgress(false);
        if (response.error_code == 0) {
          notify.success(`Cluster ${name} deleted`);
          queryClient.invalidateQueries({ queryKey: ["clusters"] });
          navigate("/ui/provisioning/clusters");
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        setDeleteInProgress(false);
        notify.error(`Error during cluster deletion: ${e}`);
      });
  };

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error || !cluster) {
    return <div>Error while loading cluster</div>;
  }

  const tabs = [
    {
      key: "overview",
      title: "Overview",
      content: <ClusterOverview />,
    },
    {
      key: "configuration",
      title: "Configuration",
      content: <ClusterConfiguration />,
    },
    {
      key: "artifacts",
      title: "Artifacts",
      content: <ClusterArtifacts />,
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
            navigate(`/ui/provisioning/clusters/${name}/${key}`)
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
        title="Delete Cluster?"
        footer={
          <>
            <LoadingButton
              isLoading={deleteInProgress}
              variant="danger"
              onClick={handleDelete}
            >
              Delete
            </LoadingButton>
          </>
        }
      >
        <div>
          <div className="mb-3">
            Are you sure you want to delete the cluster "{name}"?
            <br />
            This action cannot be undone.
          </div>
          <div className="my-3">
            <Form.Group controlId="deleteMode">
              <Form.Label>Deletion mode:</Form.Label>
              <Form.Check
                type="radio"
                id="delete-normal"
                label="Normal"
                name="deleteMode"
                value="normal"
                checked={deletionMode === "normal"}
                onChange={(e) => setDeletionMode(e.target.value as DeleteMode)}
                disabled={deleteInProgress}
              />

              <Form.Check
                type="radio"
                id="delete-force"
                label="Force"
                name="deleteMode"
                value="force"
                checked={deletionMode === "force"}
                onChange={(e) => setDeletionMode(e.target.value as DeleteMode)}
                disabled={deleteInProgress}
              />

              <Form.Check
                type="radio"
                id="delete-factory-reset"
                label="Factory reset"
                name="deleteMode"
                value="factory-reset"
                checked={deletionMode === "factory-reset"}
                onChange={(e) => setDeletionMode(e.target.value as DeleteMode)}
                disabled={deleteInProgress}
              />
            </Form.Group>
          </div>
        </div>
      </ModalWindow>
    </div>
  );
};

export default ClusterDetail;
