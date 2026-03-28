import { FC, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { MdSystemUpdateAlt } from "react-icons/md";
import { fetchServerChangelog, updateSystemServer } from "api/server";
import ChangelogView from "components/ChangelogView";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { Server } from "types/server";
import { useQueryClient } from "@tanstack/react-query";

interface Props {
  server: Server;
  recommended?: boolean;
}

const ServerUpdateBtn: FC<Props> = ({ server, recommended }) => {
  const [showModal, setShowModal] = useState(false);
  const [opInProgress, setOpInProgress] = useState(false);
  const { notify } = useNotification();
  const queryClient = useQueryClient();
  const actionStyle = {
    cursor: "pointer",
    color: recommended ? "red" : "grey",
  };

  const {
    data: changelog = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["servers", server.name, "changelog"],
    queryFn: () => fetchServerChangelog(server.name),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading changelog</div>;
  }

  const onUpdateServer = () => {
    setOpInProgress(true);
    updateSystemServer(server.name)
      .then((response) => {
        setOpInProgress(false);
        setShowModal(false);
        if (response.error_code == 0) {
          notify.success(`Server update triggered`);
          queryClient.invalidateQueries({ queryKey: ["servers"] });
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        setOpInProgress(false);
        setShowModal(false);
        notify.error(`Error during server update: ${e}`);
      });
  };

  return (
    <>
      <MdSystemUpdateAlt
        size={25}
        title="Update server"
        style={actionStyle}
        onClick={() => {
          setShowModal(true);
        }}
      />
      <ModalWindow
        show={showModal}
        scrollable
        handleClose={() => setShowModal(false)}
        title="Update server"
        footer={
          <>
            <LoadingButton
              isLoading={opInProgress}
              variant="danger"
              onClick={onUpdateServer}
            >
              Update
            </LoadingButton>
          </>
        }
      >
        <p>
          Are you sure you want to update server "{server.name}"?
          <br />
          {changelog?.prior_version}
          {" -> "}
          {changelog?.current_version}
        </p>
        <p>
          <h3>Changes</h3>
          <ChangelogView changelog={changelog ?? undefined} />
        </p>
      </ModalWindow>
    </>
  );
};

export default ServerUpdateBtn;
