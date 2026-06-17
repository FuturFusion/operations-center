import { FC, useState } from "react";
import { Button, Form } from "react-bootstrap";
import { useQueryClient } from "@tanstack/react-query";
import { uploadIncusImageVersion } from "api/image_incus";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { IncusImage } from "types/image_incus";

const architectures = ["amd64", "arm64", "armhf", "riscv64"];

interface Props {
  image?: IncusImage;
}

const UploadIncusImageBtn: FC<Props> = ({ image }) => {
  const [showModal, setShowModal] = useState(false);
  const [opInProgress, setOpInProgress] = useState(false);
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

  const name = image?.name ?? [os, release, architecture, variant].join(":");
  const isValid =
    os != "" &&
    release != "" &&
    architecture != "" &&
    variant != "" &&
    version != "" &&
    files.length > 0;

  const handleFilesChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setFiles(Array.from(event.target.files ?? []));
  };

  const onUpload = () => {
    setOpInProgress(true);
    uploadIncusImageVersion(name, version, files)
      .then((response) => {
        setOpInProgress(false);
        if (response.error_code == 0) {
          notify.success(`Version ${version} of image ${name} uploaded`);
          queryClient.invalidateQueries({ queryKey: ["incus-images"] });
          setShowModal(false);
          setVersion("");
          setFiles([]);
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
        handleClose={() => setShowModal(false)}
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
          {!image && (
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
                    <option value={arch}>{arch}</option>
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
          <Form.Group className="mb-3" controlId="version">
            <Form.Label>Version</Form.Label>
            <Form.Control
              type="text"
              placeholder="yyyymmdd"
              value={version}
              onChange={(e) => setVersion(e.target.value)}
            />
          </Form.Group>
          <Form.Group className="mb-3" controlId="files">
            <Form.Label>Files</Form.Label>
            <Form.Control type="file" multiple onChange={handleFilesChange} />
            <Form.Text muted>
              Image files, e.g. incus.tar.xz, root.tar.xz, root.squashfs,
              disk.qcow2.
            </Form.Text>
          </Form.Group>
        </Form>
      </ModalWindow>
    </>
  );
};

export default UploadIncusImageBtn;
