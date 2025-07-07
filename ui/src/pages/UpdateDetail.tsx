import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import { fetchUpdate } from "api/update";
import TabView from "components/TabView";
import UpdateFiles from "pages/UpdateFiles";
import UpdateOverview from "pages/UpdateOverview";

const UpdateDetail = () => {
  const navigate = useNavigate();
  const { uuid, activeTab } = useParams<{ uuid: string; activeTab: string }>();

  const {
    data: update = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["updates", uuid],
    queryFn: () => fetchUpdate(uuid || ""),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error || !update) {
    return <div>Error while loading update</div>;
  }

  const tabs = [
    {
      key: "overview",
      title: "Overview",
      content: <UpdateOverview />,
    },
    {
      key: "files",
      title: "Files",
      content: <UpdateFiles />,
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
            navigate(`/ui/provisioning/updates/${uuid}/${key}`)
          }
        />
      </div>
    </div>
  );
};

export default UpdateDetail;
