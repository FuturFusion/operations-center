import { FC } from "react";
import { Button } from "react-bootstrap";
import { useFormik } from "formik";
import { downloadImage } from "api/token";
import DownloadImageForm from "components/DownloadImageForm";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { Token } from "types/token";
import { DownloadImageFormValues } from "types/token";
import YAML from "yaml";

interface Props {
  token: Token;
  show: boolean;
  downloadChanged: (val: boolean) => void;
  handleClose: () => void;
}

const TokenDownloadModal: FC<Props> = ({
  token,
  show,
  downloadChanged,
  handleClose,
}) => {
  const { notify } = useNotification();
  const formikInitialValues: DownloadImageFormValues = {
    applications: ["incus"],
    type: "iso",
    install: {
      force_install: true,
      force_reboot: false,
      target: {
        id: "",
      },
    },
    network: "",
  };

  const formik = useFormik({
    initialValues: formikInitialValues,
    onSubmit: (values: DownloadImageFormValues, { resetForm }) => {
      let parsedNetwork = {};
      try {
        parsedNetwork = YAML.parse(values.network);
      } catch (error) {
        notify.error(`Error during network parsing: ${error}`);
        return;
      }

      handleClose();
      download({ ...values, network: parsedNetwork });
      resetForm();
    },
  });

  const download = async (values: object) => {
    downloadChanged(true);

    try {
      const url = await downloadImage(
        token.uuid,
        JSON.stringify(values, null, 2),
      );

      const a = document.createElement("a");
      a.href = url;
      a.download = `${token.uuid}.${(values as DownloadImageFormValues).type}`;
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
      <DownloadImageForm formik={formik} />
    </ModalWindow>
  );
};

export default TokenDownloadModal;
