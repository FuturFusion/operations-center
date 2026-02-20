import { FC, KeyboardEvent, useEffect, useState } from "react";
import { MdViewList } from "react-icons/md";
import { useQueryClient } from "@tanstack/react-query";
import { updateUpdate } from "api/update";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import ChannelMultiSelect from "components/ChannelMultiSelect";
import { useNotification } from "context/notificationContext";
import { useChannels } from "context/useChannels";
import { Update } from "types/update";
import { handleCtrlA } from "util/util";

interface Props {
  update: Update;
}

const UpdateChannelBtn: FC<Props> = ({ update }) => {
  const [showModal, setShowModal] = useState(false);
  const [opInProgress, setOpInProgress] = useState(false);
  const [channels, setChannels] = useState(update.channels);
  const { data: allChannels } = useChannels();
  const queryClient = useQueryClient();
  const { notify } = useNotification();

  const actionStyle = {
    cursor: "pointer",
    color: "grey",
  };

  useEffect(() => {
    setChannels(update.channels);
  }, [update.channels]);

  const onSubmit = () => {
    setOpInProgress(true);
    updateUpdate(update.uuid, JSON.stringify({ channels: channels }, null, 2))
      .then((response) => {
        setOpInProgress(false);
        setShowModal(false);
        if (response.error_code == 0) {
          notify.success(`Update ${update.uuid} updated`);
          queryClient.invalidateQueries({ queryKey: ["updates"] });
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        setOpInProgress(false);
        setShowModal(false);
        notify.error(`Error during Update update: ${e}`);
      });
  };

  const onChange = (values: string[]) => {
    setChannels(values);
  };

  const handleChannelsCtrlA = (e: KeyboardEvent<HTMLSelectElement>) => {
    e.preventDefault();
    setChannels(allChannels?.map((s) => s.name) ?? []);
  };

  return (
    <>
      <MdViewList
        size={25}
        title="Channels update"
        style={actionStyle}
        onClick={() => {
          setShowModal(true);
        }}
      />
      <ModalWindow
        show={showModal}
        scrollable
        handleClose={() => setShowModal(false)}
        title="Channels update"
        footer={
          <>
            <LoadingButton
              isLoading={opInProgress}
              variant="success"
              onClick={onSubmit}
            >
              Save
            </LoadingButton>
          </>
        }
      >
        <ChannelMultiSelect
          value={channels}
          onChange={onChange}
          onKeyDown={handleCtrlA(handleChannelsCtrlA)}
        />
      </ModalWindow>
    </>
  );
};

export default UpdateChannelBtn;
