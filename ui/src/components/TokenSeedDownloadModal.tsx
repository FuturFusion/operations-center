import { FC } from "react";
import { Button } from "react-bootstrap";
import { useFormik } from "formik";
import { downloadTokenSeedImage } from "api/token";
import ModalWindow from "components/ModalWindow";
import TokenSeedImageForm from "components/TokenSeedImageForm";
import { useNotification } from "context/notificationContext";
import { TokenSeed } from "types/token";
import { TokenSeedImageFormValues } from "types/token";

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
      const url = await downloadTokenSeedImage(
        seed.token_uuid || "",
        seed.name,
        values.type,
        values.architecture,
      );

      const a = document.createElement("a");
      a.href = url;
      a.download = `${seed.name}.${(values as TokenSeedImageFormValues).type}`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      window.URL.revokeObjectURL(url);
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
