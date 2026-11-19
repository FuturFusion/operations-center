import { FC, KeyboardEvent, useState } from "react";
import Button from "react-bootstrap/Button";
import Form from "react-bootstrap/Form";
import { useQueryClient } from "@tanstack/react-query";
import { downloadUpdate } from "api/update";
import ChannelMultiSelect from "components/ChannelMultiSelect";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { useChannels } from "context/useChannels";
import { errorMessage } from "util/response";
import { handleCtrlA } from "util/util";

const DownloadUpdateBtn: FC = () => {
  const [showModal, setShowModal] = useState(false);
  const [opInProgress, setOpInProgress] = useState(false);
  const [version, setVersion] = useState("");
  const [channels, setChannels] = useState<string[]>([]);
  const { data: allChannels } = useChannels();
  const queryClient = useQueryClient();
  const { notify } = useNotification();

  const openModal = () => {
    setVersion("");
    setChannels([]);
    setShowModal(true);
  };

  // The request completes after the download, so it can take a while.
  const onSubmit = () => {
    setOpInProgress(true);
    downloadUpdate(version.trim(), channels)
      .then((response) => {
        setOpInProgress(false);
        if (response.error_code == 0) {
          notify.success(`Update ${version.trim()} downloaded`);
          queryClient.invalidateQueries({ queryKey: ["updates"] });
          setShowModal(false);
          return;
        }
        notify.error(errorMessage(response));
      })
      .catch((e) => {
        setOpInProgress(false);
        notify.error(`Error during update download: ${e}`);
      });
  };

  const handleChannelsCtrlA = (e: KeyboardEvent<HTMLSelectElement>) => {
    e.preventDefault();
    setChannels(allChannels?.map((s) => s.name) ?? []);
  };

  return (
    <>
      <Button variant="success" className="float-end mx-2" onClick={openModal}>
        Download
      </Button>
      <ModalWindow
        show={showModal}
        scrollable
        handleClose={() => setShowModal(false)}
        title="Download update"
        footer={
          <>
            <LoadingButton
              isLoading={opInProgress}
              variant="success"
              disabled={version.trim() == ""}
              onClick={onSubmit}
            >
              Download
            </LoadingButton>
          </>
        }
      >
        <Form.Group className="mb-3" controlId="download-update-version">
          <Form.Label>Version</Form.Label>
          <Form.Control
            type="text"
            placeholder="202610272234"
            value={version}
            onChange={(e) => setVersion(e.target.value)}
          />
          <Form.Text className="text-muted">
            The update source must still provide this version. It keeps only a
            limited number of versions.
          </Form.Text>
        </Form.Group>
        <Form.Group controlId="download-update-channels">
          <Form.Label>Channels</Form.Label>
          <ChannelMultiSelect
            value={channels}
            onChange={setChannels}
            onKeyDown={handleCtrlA(handleChannelsCtrlA)}
          />
          <Form.Text className="text-muted">
            Without a channel, the update is assigned to the default channel. A
            refresh removes updates, which are neither in use by a server nor
            among the most recent updates of a channel. Assign an older version
            to a separate channel to keep it.
          </Form.Text>
        </Form.Group>
      </ModalWindow>
    </>
  );
};

export default DownloadUpdateBtn;
