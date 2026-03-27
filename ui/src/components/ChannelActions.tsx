import { FC } from "react";
import ChannelChangelogBtn from "components/ChannelChangelogBtn";
import { Channel } from "types/channel";

interface Props {
  channel: Channel;
}

const ChannelActions: FC<Props> = ({ channel }) => {
  return (
    <div>
      <ChannelChangelogBtn channel={channel} />
    </div>
  );
};

export default ChannelActions;
