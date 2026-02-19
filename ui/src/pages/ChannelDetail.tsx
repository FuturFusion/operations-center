import { useState } from "react";
import { Button } from "react-bootstrap";
import { useNavigate, useParams } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { deleteChannel, fetchChannel } from "api/channel";
import ModalWindow from "components/ModalWindow";
import TabView from "components/TabView";
import { useNotification } from "context/notificationContext";
import ChannelConfiguration from "pages/ChannelConfiguration";
import ChannelOverview from "pages/ChannelOverview";

const ChannelDetail = () => {
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const queryClient = useQueryClient();
  const { notify } = useNotification();
  const navigate = useNavigate();
  const { name, activeTab } = useParams<{ name: string; activeTab: string }>();

  const tabs = [
    {
      key: "overview",
      title: "Overview",
      content: <ChannelOverview />,
    },
    {
      key: "configuration",
      title: "Configuration",
      content: <ChannelConfiguration />,
    },
  ];

  const {
    data: channel = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["channels", name],
    queryFn: () => fetchChannel(name || ""),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error || !channel) {
    return <div>Error while loading channel</div>;
  }

  const handleDelete = () => {
    deleteChannel(name || "")
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Channel ${name} deleted`);
          queryClient.invalidateQueries({ queryKey: ["channels"] });
          navigate("/ui/provisioning/channels");
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during channel deletion: ${e}`);
      });
  };

  return (
    <div className="d-flex flex-column">
      <div className="scroll-container flex-grow-1 p-3">
        <TabView
          defaultTab="overview"
          activeTab={activeTab}
          tabs={tabs}
          onSelect={(key) =>
            navigate(`/ui/provisioning/channels/${name}/${key}`)
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
        title="Delete Channel?"
        footer={
          <>
            <Button variant="danger" onClick={handleDelete}>
              Delete
            </Button>
          </>
        }
      >
        <p>
          Are you sure you want to delete the channel "{name}"?
          <br />
          This action cannot be undone.
        </p>
      </ModalWindow>
    </div>
  );
};

export default ChannelDetail;
