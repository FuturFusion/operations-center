import { FC, useState } from "react";
import { Spinner } from "react-bootstrap";
import { MdDownload } from "react-icons/md";
import { checkUpdateArchive, updateArchiveURL } from "api/update";
import { useNotification } from "context/notificationContext";
import { Update } from "types/update";
import { APIError } from "util/response";
import { downloadFile } from "util/util";

interface Props {
  update: Update;
}

const UpdateExportBtn: FC<Props> = ({ update }) => {
  const [opInProgress, setOpInProgress] = useState(false);
  const { notify } = useNotification();

  if (update.update_status != "ready") {
    return null;
  }

  // A download link cannot show the error of the server, so check the export
  // first. The browser then streams the archive to disk. A fetch would hold
  // the whole archive in memory.
  const onExport = () => {
    setOpInProgress(true);
    checkUpdateArchive(update.uuid)
      .then(() => {
        downloadFile(
          updateArchiveURL(update.uuid),
          `update-${update.version}.tar`,
        );
      })
      .catch((e) => {
        if (e instanceof APIError) {
          notify.error(e.message);
          return;
        }
        notify.error(`Error during update export: ${e}`);
      })
      .finally(() => setOpInProgress(false));
  };

  if (opInProgress) {
    return (
      <Spinner
        animation="border"
        role="status"
        variant="success"
        style={{ width: "1rem", height: "1rem" }}
      />
    );
  }

  return (
    <MdDownload
      size={25}
      title="Export update"
      style={{ cursor: "pointer", color: "grey" }}
      onClick={onExport}
    />
  );
};

export default UpdateExportBtn;
