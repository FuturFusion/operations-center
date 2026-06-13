import { useState } from "react";
import { Button } from "react-bootstrap";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import { deleteIncusImage, fetchIncusImage } from "api/image_incus";
import Breadcrumbs from "components/Breadcrumbs";
import ModalWindow from "components/ModalWindow";
import TabView from "components/TabView";
import { useNotification } from "context/notificationContext";
import IncusImageOverview from "pages/IncusImageOverview";
import IncusImageConfiguration from "pages/IncusImageConfiguration";
import IncusImageVersions from "pages/IncusImageVersions";

const IncusImageDetail = () => {
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const queryClient = useQueryClient();
  const { notify } = useNotification();
  const navigate = useNavigate();
  const { name, activeTab } = useParams<{ name: string; activeTab: string }>();

  const {
    data: image = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["incus-images", name],
    queryFn: () => fetchIncusImage(name || ""),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error || !image) {
    return <div>Error while loading image</div>;
  }

  const handleDelete = () => {
    deleteIncusImage(name || "")
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Image ${name} deleted`);
          queryClient.invalidateQueries({ queryKey: ["incus-images"] });
          navigate("/ui/images-view");
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during image deletion: ${e}`);
      });
  };

  const tabs = [
    {
      key: "overview",
      title: "Overview",
      content: <IncusImageOverview />,
    },
    {
      key: "configuration",
      title: "Configuration",
      content: <IncusImageConfiguration />,
    },
    {
      key: "versions",
      title: "Versions",
      content: <IncusImageVersions />,
    },
  ];

  return (
    <div className="d-flex flex-column">
      <Breadcrumbs />
      <div className="scroll-container flex-grow-1 p-3">
        <TabView
          defaultTab="overview"
          activeTab={activeTab}
          tabs={tabs}
          onSelect={(key) => navigate(`/ui/images/incus/${name}/${key}`)}
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
        title="Delete image?"
        footer={
          <>
            <Button variant="danger" onClick={handleDelete}>
              Delete
            </Button>
          </>
        }
      >
        <p>
          Are you sure you want to delete the image "{name}"?
          <br />
          This action cannot be undone.
        </p>
      </ModalWindow>
    </div>
  );
};

export default IncusImageDetail;
