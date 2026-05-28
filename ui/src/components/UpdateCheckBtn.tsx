import type { FC } from "react";
import { MdImportExport } from "react-icons/md";
import { updateCheck } from "api/os";
import { useNotification } from "context/notificationContext";

const UpdateCheckBtn: FC = () => {
  const { notify } = useNotification();

  const handleUpdateCheck = () => {
    updateCheck()
      .then(() => {
        notify.success(`Update check`);
      })
      .catch((e) => {
        notify.error(`Update check failed: ${e}`);
      });
  };

  return (
    <MdImportExport
      size={25}
      title="Update check"
      style={{ color: "grey", cursor: "pointer" }}
      onClick={handleUpdateCheck}
    />
  );
};

export default UpdateCheckBtn;
