import { FC } from "react";
import UpdateChannelBtn from "components/UpdateChannelBtn";
import { Update } from "types/update";

interface Props {
  update: Update;
}

const UpdateActions: FC<Props> = ({ update }) => {
  return (
    <div>
      <UpdateChannelBtn update={update} />
    </div>
  );
};

export default UpdateActions;
