import { FC, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { MdUpdate } from "react-icons/md";
import { fetchChannelChangelog } from "api/channel";
import ChangelogView from "components/ChangelogView";
import ModalWindow from "components/ModalWindow";
import { Channel } from "types/channel";

interface Props {
  channel: Channel;
}

const ChannelChangelogBtn: FC<Props> = ({ channel }) => {
  const [showModal, setShowModal] = useState(false);

  const actionStyle = {
    cursor: "pointer",
    color: "grey",
  };

  const { data: changelogs = undefined } = useQuery({
    queryKey: ["channels", channel.name, "changelog"],
    queryFn: () => fetchChannelChangelog(channel.name ?? ""),
  });

  return (
    <>
      <MdUpdate
        size={25}
        title="Changelog"
        style={actionStyle}
        onClick={() => {
          setShowModal(true);
        }}
      />
      <ModalWindow
        show={showModal}
        scrollable
        handleClose={() => setShowModal(false)}
        title="Channel changelog"
      >
        <div>
          {changelogs?.map((item, index) => (
            <div className="mb-3">
              <p>Version: {item.current_version}</p>
              <ChangelogView key={index} changelog={item} />
            </div>
          ))}
        </div>
      </ModalWindow>
    </>
  );
};

export default ChannelChangelogBtn;
