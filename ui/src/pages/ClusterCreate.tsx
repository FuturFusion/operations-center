import { useNavigate, useParams } from "react-router";
import { createCluster } from "api/cluster";
import TabView from "components/TabView";
import ClusterCreateForm from "components/ClusterCreateForm";
import { useNotification } from "context/notificationContext";
import { ClusterPost } from "types/cluster";

const ClusterCreate = () => {
  const { notify } = useNotification();
  const navigate = useNavigate();
  const { activeTab } = useParams<{ activeTab: string }>();

  const onSubmit = (values: ClusterPost) => {
    return createCluster(JSON.stringify(values, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Cluster ${values.name} created`);
          navigate(`/ui/provisioning/clusters-view/clusters`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during cluster creation: ${e}`);
      });
  };

  const tabs = [
    {
      key: "manual",
      title: "Manually",
      content: <ClusterCreateForm mode="manual" onSubmit={onSubmit} />,
    },
    {
      key: "template",
      title: "From template",
      content: <ClusterCreateForm mode="template" onSubmit={onSubmit} />,
    },
  ];

  return (
    <div className="d-flex flex-column">
      <div className="scroll-container flex-grow-1 p-3">
        <TabView
          defaultTab="manual"
          activeTab={activeTab}
          tabs={tabs}
          onSelect={(key) =>
            navigate(`/ui/provisioning/clusters/create/${key}`)
          }
        />
      </div>
    </div>
  );
};

export default ClusterCreate;
