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
  const [files, setFiles] = useState<File[]>([]);
  const queryClient = useQueryClient();
  const { notify } = useNotification();

  const hasIncusTarXZ = files.some((file) => file.name == "incus.tar.xz");
  const fullValid = files.length > 0 && hasIncusTarXZ;
  const metadataValid =
    os != "" &&
    release != "" &&
    architecture != "" &&
    variant != "" &&
    version != "" &&
    files.length > 0 &&
    !hasIncusTarXZ;
  const isValid = mode == "full" ? fullValid : metadataValid;

  const reset = () => {
    setShowModal(false);
    setVersion("");
    setFiles([]);
  };

  const handleFilesChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setFiles(Array.from(event.target.files ?? []));
  };

  const doUpload = () => {
    if (mode == "full") {
      return uploadIncusImageFull(files);
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
        notify.error(response.error);
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
              label="Upload complete image files (including incus.tar.xz)"
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
          <Form.Group className="mb-3" controlId="files">
            <Form.Label>Files</Form.Label>
            <Form.Control type="file" multiple onChange={handleFilesChange} />
            <Form.Text muted>
              {mode == "full"
                ? "Image files including incus.tar.xz, e.g. incus.tar.xz, root.tar.xz, root.squashfs, disk.qcow2."
                : "Image files only, e.g. root.tar.xz, root.squashfs, disk.qcow2. The incus.tar.xz is generated from the metadata."}
            </Form.Text>
          </Form.Group>
        </Form>
      </ModalWindow>
    </>
  );
};

export default UploadIncusImageBtn;
