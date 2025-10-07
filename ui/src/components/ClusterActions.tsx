import { FC, useState } from "react";
import { PiCertificate } from "react-icons/pi";
import ClusterUpdateCertModal from "components/ClusterUpdateCertModal";
import { Cluster } from "types/cluster";

interface Props {
  cluster: Cluster;
}

const ClusterActions: FC<Props> = ({ cluster }) => {
  const [showUpdateCertModal, setShowUpdateCertModal] = useState(false);
  const updateCertStyle = {
    cursor: "pointer",
    color: "grey",
  };

  const onCertUpdate = () => {
    setShowUpdateCertModal(true);
  };

  return (
    <div>
      <PiCertificate
        size={25}
        title="Update certificate"
        style={updateCertStyle}
        onClick={() => {
          onCertUpdate();
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
