import { FC, useState } from "react";
import { MdOutlineFileDownload, MdOutlineSync } from "react-icons/md";
import { PiCertificate } from "react-icons/pi";
import { downloadArtifact, resyncClusterInventory } from "api/cluster";
import ClusterUpdateCertModal from "components/ClusterUpdateCertModal";
import { useNotification } from "context/notificationContext";
import { Cluster } from "types/cluster";
import { downloadFile } from "util/util";

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
      const artifactName = "terraform-configuration";
      const url = await downloadArtifact(cluster.name || "", artifactName);

      const filename = `${cluster.name}-${artifactName}.zip`;

      downloadFile(url, filename);
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
