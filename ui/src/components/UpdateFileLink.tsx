import { FC } from "react";
import { useNotification } from "context/notificationContext";
import { downloadUpdateFile } from "api/update";
import { downloadFile } from "util/util";

type Props = {
  uuid: string | undefined;
  filename: string;
};

const UpdateFileLink: FC<Props> = ({ uuid, filename }) => {
  const { notify } = useNotification();

  const handleDownload = async () => {
    try {
      const url = await downloadUpdateFile(uuid ?? "", filename);

      downloadFile(url, filename);
    } catch (error) {
      notify.error(`Error during image downloading: ${error}`);
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

export default UpdateFileLink;
