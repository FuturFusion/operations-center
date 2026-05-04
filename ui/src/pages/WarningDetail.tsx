import Button from "react-bootstrap/Button";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import { fetchWarning, updateWarning } from "api/warnings";
import Breadcrumbs from "components/Breadcrumbs";
import WarningOverview from "components/WarningOverview";
import TabView from "components/TabView";
import { useNotification } from "context/notificationContext";
import { WarningStatus } from "util/warning";

const WarningDetail = () => {
  const { notify } = useNotification();
  const { uuid, activeTab } = useParams<{ uuid: string; activeTab: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const {
    data: warning = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["warnings", uuid],
    queryFn: () => fetchWarning(uuid),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading warning</div>;
  }

  const handleAcknowledge = () => {
    updateWarning(
      uuid ?? "",
      JSON.stringify({ status: WarningStatus.Acknowledged }, null, 2),
    )
      .then((response) => {
        if (response.error_code == 0) {
          void queryClient.invalidateQueries({ queryKey: ["warnings", uuid] });
          notify.success(`Warning ${uuid} acknowledged`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error while acknowledging warning: ${e}`);
      });
  };

  const tabs = [
    {
      key: "overview",
      title: "Overview",
      content: <WarningOverview />,
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
          onSelect={(key) =>
            navigate(`/ui/provisioning/warnings/${uuid}/${key}`)
          }
        />
      </div>
      <div className="fixed-footer p-3">
        {(!activeTab || activeTab == "overview") &&
          warning?.status == WarningStatus.New && (
            <Button
              className="float-end"
              variant="success"
              onClick={handleAcknowledge}
            >
              Acknowledge
            </Button>
          )}
      </div>
    </div>
  );
};

export default WarningDetail;
