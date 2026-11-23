import { FC } from "react";
import UpdateChannelBtn from "components/UpdateChannelBtn";
import UpdateExportBtn from "components/UpdateExportBtn";
import { Update } from "types/update";

interface Props {
  update: Update;
}

const UpdateActions: FC<Props> = ({ update }) => {
  return (
    <div>
      <UpdateChannelBtn update={update} />
      <UpdateExportBtn update={update} />
    </div>
  );
};

export default UpdateActions;
