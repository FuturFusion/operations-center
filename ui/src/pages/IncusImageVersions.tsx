import { useState } from "react";
import Button from "react-bootstrap/Button";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams } from "react-router";
import { BsHash } from "react-icons/bs";
import { deleteIncusImageVersion, fetchIncusImage } from "api/image_incus";
import DataTable from "components/DataTable";
import IncusImageFileLink from "components/IncusImageFileLink";
import ModalWindow from "components/ModalWindow";
import UploadIncusImageBtn from "components/UploadIncusImageBtn";
import { useNotification } from "context/notificationContext";
import { localImageSourceName } from "util/image_source";
import { bytesToHumanReadable } from "util/util";
import { errorMessage } from "util/response";

const IncusImageVersions = () => {
  const { source, name } = useParams() as { source: string; name: string };
  const [versionToDelete, setVersionToDelete] = useState("");
  const queryClient = useQueryClient();
  const { notify } = useNotification();

  const {
    data: image = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["incus-images", source, name],
    queryFn: () => fetchIncusImage(source, name),
  });

  if (isLoading) {
    return <div>Loading versions...</div>;
  }

  if (error || !image) {
    return <div>Error while loading versions</div>;
  }

  const handleDeleteVersion = () => {
    const version = versionToDelete;
    setVersionToDelete("");

    deleteIncusImageVersion(source, name, version)
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Version ${version} of image ${name} deleted`);
          queryClient.invalidateQueries({ queryKey: ["incus-images", source] });
          return;
        }
        notify.error(errorMessage(response));
      })
      .catch((e) => {
        notify.error(`Error during version deletion: ${e}`);
      });
  };

  // Only the images of the local source can be changed.
  const isLocal = source == localImageSourceName;
  const headers = ["Filename", "Size", "Type", ""];
  const versions = Object.keys(image.versions ?? {})
    .sort()
    .reverse();

  return (
    <>
      <div className="d-flex flex-column">
        {isLocal && (
          <div className="mx-2 mx-md-4 mb-3">
            <div className="row">
              <div className="col-12">
                <UploadIncusImageBtn image={image} />
              </div>
            </div>
          </div>
        )}
        {versions.map((version) => {
          const items = image.versions[version].items ?? {};
          const rows = Object.keys(items)
            .sort()
            .map((filename) => {
              const item = items[filename];
              return {
                cols: [
                  {
                    content: (
                      <IncusImageFileLink
                        source={source}
                        name={name}
                        version={version}
                        filename={filename}
                      />
                    ),
                    sortKey: filename,
                  },
                  {
                    content: bytesToHumanReadable(item.size),
                    sortKey: item.size,
                  },
                  {
                    content: item.ftype,
                    sortKey: item.ftype,
                  },
                  {
                    content: item.sha256 ? (
                      <BsHash
                        title={item.sha256}
                        style={{ cursor: "pointer" }}
                      />
                    ) : (
                      ""
                    ),
                    sortKey: item.sha256,
                  },
                ],
              };
            });

          return (
            <div className="mb-4">
              <div className="mx-2 mx-md-4 d-flex align-items-center">
                <h5 className="flex-grow-1 my-0">{version}</h5>
                {isLocal && (
                  <Button
                    variant="danger"
                    size="sm"
                    onClick={() => setVersionToDelete(version)}
                  >
                    Delete
                  </Button>
                )}
              </div>
              <DataTable headers={headers} rows={rows} />
            </div>
          );
        })}
        {versions.length == 0 && (
          <div className="mx-2 mx-md-4">No versions available.</div>
        )}
      </div>
      <ModalWindow
        show={versionToDelete != ""}
        handleClose={() => setVersionToDelete("")}
        title="Delete version?"
        footer={
          <>
            <Button variant="danger" onClick={handleDeleteVersion}>
              Delete
            </Button>
          </>
        }
      >
        <p>
          Are you sure you want to delete version "{versionToDelete}" of image "
          {name}"?
          <br />
          This action cannot be undone.
        </p>
      </ModalWindow>
    </>
  );
};

export default IncusImageVersions;
