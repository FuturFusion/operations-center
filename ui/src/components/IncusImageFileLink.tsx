import { FC } from "react";
import { useNotification } from "context/notificationContext";
import { downloadIncusImageFile } from "api/image_incus";
import { downloadFile } from "util/util";

type Props = {
  name: string;
  version: string;
  filename: string;
};

const IncusImageFileLink: FC<Props> = ({ name, version, filename }) => {
  const { notify } = useNotification();

  const handleDownload = async () => {
    try {
      const url = await downloadIncusImageFile(name, version, filename);

      downloadFile(url, filename);
    } catch (error) {
      notify.error(`Error during file downloading: ${error}`);
    }
  };

  return (
    <a
      onClick={(e) => {
        e.preventDefault();
        handleDownload();
      }}
      className="data-table-link"
      style={{ cursor: "pointer" }}
      title={filename}
    >
      {filename.length > 30 ? `${filename.slice(0, 30)}...` : filename}
    </a>
  );
};

export default IncusImageFileLink;
