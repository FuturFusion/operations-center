import { FC, useState } from "react";
import { Spinner } from "react-bootstrap";
import { MdOutlineFileDownload } from "react-icons/md";
import TokenSeedDownloadModal from "components/TokenSeedDownloadModal";
import { TokenSeed } from "types/token";

interface Props {
  seed: TokenSeed;
}

const TokenSeedActions: FC<Props> = ({ seed }) => {
  const [showDownloadModal, setShowDownloadModal] = useState(false);
  const [downloadInProgress, setDownloadInProgress] = useState(false);
  const downloadStyle = {
    cursor: "pointer",
    color: "grey",
  };

  const onDownload = () => {
    setShowDownloadModal(true);
  };

  return (
    <div>
      {!downloadInProgress && (
        <MdOutlineFileDownload
          size={25}
          style={downloadStyle}
          onClick={() => {
            onDownload();
          }}
        />
      )}
      {downloadInProgress && (
        <Spinner
          animation="border"
          role="status"
          variant="success"
          style={{ width: "1rem", height: "1rem" }}
        />
      )}
      <TokenSeedDownloadModal
        seed={seed}
        downloadChanged={(val) => setDownloadInProgress(val)}
        show={showDownloadModal}
        handleClose={() => setShowDownloadModal(false)}
      />
    </div>
  );
};

export default TokenSeedActions;
