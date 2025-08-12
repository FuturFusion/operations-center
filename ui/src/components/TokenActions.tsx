import { FC, useState } from "react";
import { Spinner } from "react-bootstrap";
import { MdOutlineFileDownload } from "react-icons/md";
import TokenDownloadModal from "components/TokenDownloadModal";
import { Token } from "types/token";

interface Props {
  token: Token;
}

const TokenActions: FC<Props> = ({ token }) => {
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
      <TokenDownloadModal
        token={token}
        downloadChanged={(val) => setDownloadInProgress(val)}
        show={showDownloadModal}
        handleClose={() => setShowDownloadModal(false)}
      />
    </div>
  );
};

export default TokenActions;
