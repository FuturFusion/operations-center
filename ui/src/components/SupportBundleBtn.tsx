import { FC, useState } from "react";
import { Button } from "react-bootstrap";
import { MdOutlineSupport } from "react-icons/md";
import { createSupportBundle } from "api/support_bundle";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { SupportBundlePost } from "types/support_bundle";
import { downloadFile } from "util/util";

interface Props {
  target: SupportBundlePost;
  asButton?: boolean;
}

const describeTarget = (target: SupportBundlePost): string => {
  switch (target.type) {
    case "cluster":
      return `cluster "${target.cluster}"`;
    case "project":
      return `project "${target.project}" of cluster "${target.cluster}"`;
    case "instance":
      return `instance "${target.name}" in project "${target.project}" of cluster "${target.cluster}"`;
    case "server":
      return `server "${target.server}"`;
  }
};

const SupportBundleBtn: FC<Props> = ({ target, asButton }) => {
  const [showModal, setShowModal] = useState(false);
  const [opInProgress, setOpInProgress] = useState(false);
  const { notify } = useNotification();
  const actionStyle = {
    cursor: "pointer",
    color: "grey",
  };

  const onCreate = () => {
    setOpInProgress(true);
    createSupportBundle(target)
      .then(({ url, filename }) => {
        downloadFile(url, filename);
        notify.success("Support bundle created");
        setShowModal(false);
      })
      .catch((e) => {
        notify.error(`Error during support bundle creation: ${e}`);
      })
      .finally(() => {
        setOpInProgress(false);
      });
  };

  return (
    <>
      {asButton ? (
        <Button variant="outline-secondary" onClick={() => setShowModal(true)}>
          Support bundle
        </Button>
      ) : (
        <MdOutlineSupport
          size={25}
          title="Create support bundle"
          style={actionStyle}
          onClick={() => setShowModal(true)}
        />
      )}
      <ModalWindow
        show={showModal}
        handleClose={() => setShowModal(false)}
        title="Create support bundle"
        footer={
          <>
            <Button
              variant="secondary"
              onClick={() => setShowModal(false)}
              disabled={opInProgress}
            >
              Cancel
            </Button>
            <LoadingButton
              isLoading={opInProgress}
              variant="success"
              onClick={onCreate}
            >
              Create and download
            </LoadingButton>
          </>
        }
      >
        <p>Create a support bundle for the {describeTarget(target)}.</p>
        <p>
          The bundle is a zip archive with the records of Operations Center, the
          configuration and state from Incus and IncusOS and the IncusOS journal
          and logs of the involved servers. Data, which can not be collected, is
          replaced by an error file in the archive.
        </p>
        <p>
          Secrets in the configuration are redacted, logs are included as is.
          Collecting the data can take several minutes.
        </p>
      </ModalWindow>
    </>
  );
};

export default SupportBundleBtn;
