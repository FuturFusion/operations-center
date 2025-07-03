import { useState } from "react";
import { Button } from "react-bootstrap";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import { deleteToken, fetchToken } from "api/token";
import ModalWindow from "components/ModalWindow";
import TabView from "components/TabView";
import { useNotification } from "context/notificationContext";
import TokenOverview from "pages/TokenOverview";
import TokenConfiguration from "pages/TokenConfiguration";

const TokenDetail = () => {
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const queryClient = useQueryClient();
  const { notify } = useNotification();
  const navigate = useNavigate();
  const { uuid, activeTab } = useParams<{ uuid: string; activeTab: string }>();

  const {
    data: token = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["tokens", uuid],
    queryFn: () => fetchToken(uuid || ""),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error || !token) {
    return <div>Error while loading token</div>;
  }

  const handleDelete = () => {
    deleteToken(uuid || "")
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Token ${uuid} deleted`);
          queryClient.invalidateQueries({ queryKey: ["tokens"] });
          navigate("/ui/provisioning/tokens");
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during token deletion: ${e}`);
      });
  };

  const tabs = [
    {
      key: "overview",
      title: "Overview",
      content: <TokenOverview />,
    },
    {
      key: "configuration",
      title: "Configuration",
      content: <TokenConfiguration />,
    },
  ];

  return (
    <div className="d-flex flex-column">
      <div className="scroll-container flex-grow-1 p-3">
        <TabView
          defaultTab="overview"
          activeTab={activeTab}
          tabs={tabs}
          onSelect={(key) => navigate(`/ui/provisioning/tokens/${uuid}/${key}`)}
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
        title="Delete Token?"
        footer={
          <>
            <Button variant="danger" onClick={handleDelete}>
              Delete
            </Button>
          </>
        }
      >
        <p>
          Are you sure you want to delete the token "{uuid}"?
          <br />
          This action cannot be undone.
        </p>
      </ModalWindow>
    </div>
  );
};

export default TokenDetail;
