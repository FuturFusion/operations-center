import { FC } from "react";
import { Button } from "react-bootstrap";
import { useFormik } from "formik";
import ModalWindow from "components/ModalWindow";
import TokenSeedImageForm from "components/TokenSeedImageForm";
import { useNotification } from "context/notificationContext";
import { TokenSeed } from "types/token";
import { TokenSeedImageFormValues } from "types/token";
import { downloadFile } from "util/util";

interface Props {
  seed: TokenSeed;
  show: boolean;
  downloadChanged: (val: boolean) => void;
  handleClose: () => void;
}

const TokenSeedDownloadModal: FC<Props> = ({
  seed,
  show,
  downloadChanged,
  handleClose,
}) => {
  const { notify } = useNotification();
  const formikInitialValues: TokenSeedImageFormValues = {
    type: "iso",
    architecture: "x86_64",
  };

  const formik = useFormik({
    initialValues: formikInitialValues,
    onSubmit: (values: TokenSeedImageFormValues, { resetForm }) => {
      handleClose();
      download(values);
      resetForm();
    },
  });

  const download = async (values: TokenSeedImageFormValues) => {
    downloadChanged(true);

    try {
      const tokenUUID = seed.token_uuid || "";
      // Parameters and the terminal filename are encoded as path segments, so
      // that the URL ends in a recognized media extension.
      const url = `/1.0/provisioning/tokens/${tokenUUID}/seeds/${encodeURIComponent(seed.name)}/architecture/${values.architecture}/type/${values.type}/file.${values.type}`;
      const filename = `${seed.name}.${(values as TokenSeedImageFormValues).type}`;

      downloadFile(url, filename);
    } catch (error) {
      notify.error(`Error during image downloading: ${error}`);
    }

    downloadChanged(false);
  };

  return (
    <ModalWindow
      show={show}
      handleClose={handleClose}
      title="Download image"
      footer={
        <>
          <Button variant="success" onClick={formik.submitForm}>
            Download
          </Button>
        </>
      }
    >
      <TokenSeedImageForm formik={formik} />
    </ModalWindow>
  );
};

export default TokenSeedDownloadModal;
