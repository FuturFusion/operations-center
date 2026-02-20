import { useState } from "react";
import { Button } from "react-bootstrap";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import {
  deleteClusterTemplate,
  fetchClusterTemplate,
} from "api/cluster_template";
import ModalWindow from "components/ModalWindow";
import TabView from "components/TabView";
import { useNotification } from "context/notificationContext";
import ClusterTemplateOverview from "pages/ClusterTemplateOverview";
import ClusterTemplateConfiguration from "pages/ClusterTemplateConfiguration";

const ClusterTemplateDetail = () => {
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const queryClient = useQueryClient();
  const { notify } = useNotification();
  const navigate = useNavigate();
  const { name, activeTab } = useParams<{ name: string; activeTab: string }>();

  const {
    data: template = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["cluster-templates", name],
    queryFn: () => fetchClusterTemplate(name || ""),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error || !template) {
    return <div>Error while loading cluster template</div>;
  }

  const handleDelete = () => {
    deleteClusterTemplate(name || "")
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Cluster template ${name} deleted`);
          queryClient.invalidateQueries({ queryKey: ["cluster-templates"] });
          navigate("/ui/provisioning/clusters-view/templates");
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during cluster template deletion: ${e}`);
      });
  };

  const tabs = [
    {
      key: "overview",
      title: "Overview",
      content: <ClusterTemplateOverview />,
    },
    {
      key: "configuration",
      title: "Configuration",
      content: <ClusterTemplateConfiguration />,
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
            navigate(`/ui/provisioning/cluster-templates/${name}/${key}`)
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
        title="Delete Cluster template?"
        footer={
          <>
            <Button variant="danger" onClick={handleDelete}>
              Delete
            </Button>
          </>
        }
      >
        <p>
          Are you sure you want to delete the cluster template "{name}"?
          <br />
          This action cannot be undone.
        </p>
      </ModalWindow>
    </div>
  );
};

export default ClusterTemplateDetail;
