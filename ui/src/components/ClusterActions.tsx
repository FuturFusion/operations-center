import { FC, useState } from "react";
import { MdOutlineFileDownload, MdOutlineSync } from "react-icons/md";
import { PiCertificate } from "react-icons/pi";
import { downloadTerraformData, resyncClusterInventory } from "api/cluster";
import ClusterUpdateCertModal from "components/ClusterUpdateCertModal";
import { useNotification } from "context/notificationContext";
import { Cluster } from "types/cluster";

interface Props {
  cluster: Cluster;
}

const ClusterActions: FC<Props> = ({ cluster }) => {
  const { notify } = useNotification();
  const [showUpdateCertModal, setShowUpdateCertModal] = useState(false);
  const actionStyle = {
    cursor: "pointer",
    color: "grey",
  };

  const onCertUpdate = () => {
    setShowUpdateCertModal(true);
  };

  const onDownloadTerraformData = async () => {
    try {
      const url = await downloadTerraformData(cluster.name);

      const a = document.createElement("a");
      a.href = url;
      a.download = `${cluster.name}-terraform-configuration.zip`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      window.URL.revokeObjectURL(url);
    } catch (error) {
      notify.error(`Error during terraform data downloading: ${error}`);
    }
  };

  const onResyncClusterInventory = () => {
    resyncClusterInventory(cluster.name)
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Cluster inventory resync triggered`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during cluster inventory sync: ${e}`);
      });
  };

  return (
    <div>
      <PiCertificate
        size={25}
        title="Update certificate"
        style={actionStyle}
        onClick={() => {
          onCertUpdate();
        }}
      />
      <MdOutlineFileDownload
        size={25}
        title="Download terraform data"
        style={actionStyle}
        onClick={() => {
          onDownloadTerraformData();
        }}
      />
      <MdOutlineSync
        size={25}
        title="Resync cluster inventory"
        style={actionStyle}
        onClick={() => {
          onResyncClusterInventory();
        }}
      />
      <ClusterUpdateCertModal
        cluster={cluster}
        show={showUpdateCertModal}
        handleClose={() => setShowUpdateCertModal(false)}
      />
    </div>
  );
};

export default ClusterActions;
