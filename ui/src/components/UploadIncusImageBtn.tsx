import { FC, useState } from "react";
import { Button, Form } from "react-bootstrap";
import { useQueryClient } from "@tanstack/react-query";
import {
  uploadIncusImageFull,
  uploadIncusImageWithMetadata,
} from "api/image_incus";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { IncusImage } from "types/image_incus";
import { errorMessage } from "util/response";

const architectures = ["amd64", "arm64", "armhf", "riscv64"];

type UploadMode = "full" | "metadata";

interface Props {
  image?: IncusImage;
}

const UploadIncusImageBtn: FC<Props> = ({ image }) => {
  const [showModal, setShowModal] = useState(false);
  const [opInProgress, setOpInProgress] = useState(false);
  const [mode, setMode] = useState<UploadMode>("full");
  const [os, setOs] = useState(image?.os ?? "");
  const [release, setRelease] = useState(image?.release ?? "");
  const [architecture, setArchitecture] = useState(
    image?.arch ?? architectures[0],
  );
  const [variant, setVariant] = useState(image?.variant ?? "");
  const [version, setVersion] = useState("");
  const [metadataFile, setMetadataFile] = useState<File | null>(null);
  const [files, setFiles] = useState<File[]>([]);
  const queryClient = useQueryClient();
  const { notify } = useNotification();

  const fullValid = metadataFile != null && files.length > 0;
  const metadataValid =
    os != "" &&
    release != "" &&
    architecture != "" &&
    variant != "" &&
    version != "" &&
    files.length > 0;
  const isValid = mode == "full" ? fullValid : metadataValid;

  const reset = () => {
    setShowModal(false);
    setVersion("");
    setMetadataFile(null);
    setFiles([]);
  };

  const handleMetadataFileChange = (
    event: React.ChangeEvent<HTMLInputElement>,
  ) => {
    setMetadataFile(event.target.files?.[0] ?? null);
  };

  const handleFilesChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setFiles(Array.from(event.target.files ?? []));
  };

  const doUpload = () => {
    if (mode == "full") {
      if (metadataFile == null) {
        return Promise.reject(new Error("No metadata tarball selected"));
      }

      return uploadIncusImageFull(metadataFile, files);
    }

    return uploadIncusImageWithMetadata(
      {
        os: os,
        release: release,
        arch: architecture,
        variant: variant,
        version: version,
      },
      files,
    );
  };

  const onUpload = () => {
    setOpInProgress(true);

    doUpload()
      .then((response) => {
        setOpInProgress(false);
        if (response.error_code == 0) {
          notify.success(`Image uploaded`);
          queryClient.invalidateQueries({ queryKey: ["incus-images"] });
          reset();
          return;
        }
        notify.error(errorMessage(response));
      })
      .catch((e) => {
        setOpInProgress(false);
        notify.error(`Error during image upload: ${e}`);
      });
  };

  return (
    <>
      <Button
        variant="success"
        className="float-end mx-2"
        onClick={() => setShowModal(true)}
      >
        Upload
      </Button>
      <ModalWindow
        show={showModal}
        handleClose={reset}
        title={image ? `Upload version for ${image.name}` : "Upload image"}
        footer={
          <>
            <LoadingButton
              variant="success"
              isLoading={opInProgress}
              disabled={!isValid}
              onClick={onUpload}
            >
              Upload
            </LoadingButton>
          </>
        }
      >
        <Form noValidate>
          <Form.Group className="mb-3" controlId="mode">
            <Form.Check
              type="radio"
              name="mode"
              id="mode-full"
              label="Upload metadata tarball and image files"
              checked={mode == "full"}
              onChange={() => setMode("full")}
            />
            <Form.Check
              type="radio"
              name="mode"
              id="mode-metadata"
              label="Provide metadata and upload image files"
              checked={mode == "metadata"}
              onChange={() => setMode("metadata")}
            />
          </Form.Group>
          {mode == "metadata" && !image && (
            <>
              <Form.Group className="mb-3" controlId="os">
                <Form.Label>Operating system</Form.Label>
                <Form.Control
                  type="text"
                  value={os}
                  onChange={(e) => setOs(e.target.value)}
                />
              </Form.Group>
              <Form.Group className="mb-3" controlId="release">
                <Form.Label>Release</Form.Label>
                <Form.Control
                  type="text"
                  value={release}
                  onChange={(e) => setRelease(e.target.value)}
                />
              </Form.Group>
              <Form.Group className="mb-3" controlId="architecture">
                <Form.Label>Architecture</Form.Label>
                <Form.Select
                  value={architecture}
                  onChange={(e) => setArchitecture(e.target.value)}
                >
                  {architectures.map((arch) => (
                    <option key={arch} value={arch}>
                      {arch}
                    </option>
                  ))}
                </Form.Select>
              </Form.Group>
              <Form.Group className="mb-3" controlId="variant">
                <Form.Label>Variant</Form.Label>
                <Form.Control
                  type="text"
                  value={variant}
                  onChange={(e) => setVariant(e.target.value)}
                />
              </Form.Group>
            </>
          )}
          {mode == "metadata" && (
            <Form.Group className="mb-3" controlId="version">
              <Form.Label>Version</Form.Label>
              <Form.Control
                type="text"
                value={version}
                onChange={(e) => setVersion(e.target.value)}
              />
            </Form.Group>
          )}
          {mode == "full" && (
            <Form.Group className="mb-3" controlId="metadataFile">
              <Form.Label>Metadata tarball</Form.Label>
              <Form.Control type="file" onChange={handleMetadataFileChange} />
              <Form.Text muted>
                The metadata tarball of the image, e.g. incus.tar.xz or the file
                written by &quot;incus image export&quot;. The name does not
                matter, the file is recognized by its content.
              </Form.Text>
            </Form.Group>
          )}
          <Form.Group className="mb-3" controlId="files">
            <Form.Label>Image files</Form.Label>
            <Form.Control type="file" multiple onChange={handleFilesChange} />
            <Form.Text muted>
              {mode == "full"
                ? "The image files, e.g. root.tar.xz, root.squashfs, disk.qcow2. Their type is detected from their content, the names do not matter."
                : "The image files, e.g. root.tar.xz, root.squashfs, disk.qcow2. The metadata tarball is generated from the metadata above."}
            </Form.Text>
          </Form.Group>
        </Form>
      </ModalWindow>
    </>
  );
};

export default UploadIncusImageBtn;
