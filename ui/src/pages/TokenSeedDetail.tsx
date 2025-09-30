import { useState } from "react";
import { Button } from "react-bootstrap";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import { deleteTokenSeed, fetchTokenSeed } from "api/token";
import ModalWindow from "components/ModalWindow";
import TabView from "components/TabView";
import { useNotification } from "context/notificationContext";
import TokenSeedOverview from "pages/TokenSeedOverview";
import TokenSeedConfiguration from "pages/TokenSeedConfiguration";

const TokenSeedDetail = () => {
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const queryClient = useQueryClient();
  const { notify } = useNotification();
  const navigate = useNavigate();
  const { uuid, name, activeTab } = useParams<{
    uuid: string;
    name: string;
    activeTab: string;
  }>();

  const {
    data: seed = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["tokens", uuid, "seeds", name],
    queryFn: () => fetchTokenSeed(uuid || "", name || ""),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error || !seed) {
    return <div>Error while loading token seed</div>;
  }

  const handleDelete = () => {
    deleteTokenSeed(uuid || "", name || "")
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Token seed ${uuid} deleted`);
          queryClient.invalidateQueries({
            queryKey: ["tokens", uuid, "seeds"],
          });
          navigate(`/ui/provisioning/tokens/${uuid}/seeds`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during token seed deletion: ${e}`);
      });
  };

  const tabs = [
    {
      key: "overview",
      title: "Overview",
      content: <TokenSeedOverview />,
    },
    {
      key: "configuration",
      title: "Configuration",
      content: <TokenSeedConfiguration />,
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
            navigate(`/ui/provisioning/tokens/${uuid}/seeds/${name}/${key}`)
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
        title="Delete Token Seed?"
        footer={
          <>
            <Button variant="danger" onClick={handleDelete}>
              Delete
            </Button>
          </>
        }
      >
        <p>
          Are you sure you want to delete the token seed "{name}"?
          <br />
          This action cannot be undone.
        </p>
      </ModalWindow>
    </div>
  );
};

export default TokenSeedDetail;
