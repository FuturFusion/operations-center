import { useState } from "react";
import { Button } from "react-bootstrap";
import { useNavigate, useParams } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  deleteImageSource,
  fetchImageSource,
  refreshImageSource,
} from "api/image_incus_source";
import Breadcrumbs from "components/Breadcrumbs";
import ModalWindow from "components/ModalWindow";
import TabView from "components/TabView";
import { useNotification } from "context/notificationContext";
import ImageSourceConfiguration from "pages/ImageSourceConfiguration";
import ImageSourceOverview from "pages/ImageSourceOverview";

const ImageSourceDetail = () => {
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const queryClient = useQueryClient();
  const { notify } = useNotification();
  const navigate = useNavigate();
  const { name, activeTab } = useParams<{ name: string; activeTab: string }>();

  const tabs = [
    {
      key: "overview",
      title: "Overview",
      content: <ImageSourceOverview />,
    },
    {
      key: "configuration",
      title: "Configuration",
      content: <ImageSourceConfiguration />,
    },
  ];

  const {
    data: source = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["image-sources", name],
    queryFn: () => fetchImageSource(name || ""),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error || !source) {
    return <div>Error while loading image source</div>;
  }

  const handleRefresh = () => {
    refreshImageSource(name || "")
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Refresh of image source ${name} triggered`);
          queryClient.invalidateQueries({ queryKey: ["image-sources"] });
          queryClient.invalidateQueries({ queryKey: ["incus-images"] });
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during image source refresh: ${e}`);
      });
  };

  const handleDelete = () => {
    deleteImageSource(name || "")
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Image source ${name} deleted`);
          queryClient.invalidateQueries({ queryKey: ["image-sources"] });
          navigate("/ui/images-view/sources");
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during image source deletion: ${e}`);
      });
  };

  return (
    <div className="d-flex flex-column">
      <Breadcrumbs />
      <div className="scroll-container flex-grow-1 p-3">
        <TabView
          defaultTab="overview"
          activeTab={activeTab}
          tabs={tabs}
          onSelect={(key) => navigate(`/ui/images/sources/${name}/${key}`)}
        />
      </div>
      <div className="fixed-footer p-3">
        {(!activeTab || activeTab == "overview") && (
          <div className="d-flex justify-content-end gap-2">
            <Button variant="secondary" onClick={handleRefresh}>
              Refresh
            </Button>
            <Button variant="danger" onClick={() => setShowDeleteModal(true)}>
              Delete
            </Button>
          </div>
        )}
      </div>
      <ModalWindow
        show={showDeleteModal}
        handleClose={() => setShowDeleteModal(false)}
        title="Delete image source?"
        footer={
          <>
            <Button variant="danger" onClick={handleDelete}>
              Delete
            </Button>
          </>
        }
      >
        <p>
          Are you sure you want to delete the image source "{name}"?
          <br />
          This action cannot be undone.
        </p>
      </ModalWindow>
    </div>
  );
};

export default ImageSourceDetail;
