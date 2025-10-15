import { FC } from "react";
import { Button } from "react-bootstrap";
import { FormikErrors, useFormik } from "formik";
import { downloadImage } from "api/token";
import TokenImageForm from "components/TokenImageForm";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { Token } from "types/token";
import { TokenImageFormValues } from "types/token";
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
  const formikInitialValues: TokenImageFormValues = {
    architecture: "x86_64",
    type: "iso",
    seeds: {
      application: "",
      migration_manager: "",
      operations_center: "",
      install: {
        force_install: true,
        force_reboot: false,
        target: {
          id: "",
        },
      },
      network: "",
    },
  };

  const validateForm = (
    values: TokenImageFormValues,
  ): FormikErrors<TokenImageFormValues> => {
    const errors: FormikErrors<TokenImageFormValues> = {};

    if (!values.seeds.application) {
      errors.seeds ??= {};
      errors.seeds.application = "Application is required";
    }

    return errors;
  };

  const formik = useFormik({
    initialValues: formikInitialValues,
    validate: validateForm,
    onSubmit: (values: TokenImageFormValues, { resetForm }) => {
      let parsedNetwork = null;
      let parsedMigrationManager = null;
      let parsedOperationsCenter = null;
      try {
        parsedNetwork = YAML.parse(values.seeds.network);
        parsedMigrationManager = YAML.parse(values.seeds.migration_manager);
        parsedOperationsCenter = YAML.parse(values.seeds.operations_center);
      } catch (error) {
        notify.error(`Error during YAML parsing: ${error}`);
        return;
      }

      handleClose();
      download({
        ...values,
        seeds: {
          applications: { aplications: [{ name: values.seeds.application }] },
          install: values.seeds.install,
          network: parsedNetwork,
          migration_manager: parsedMigrationManager,
          operations_center: parsedOperationsCenter,
        },
      });
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
      a.download = `${token.uuid}.${(values as TokenImageFormValues).type}`;
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
      <TokenImageForm formik={formik} />
    </ModalWindow>
  );
};

export default TokenDownloadModal;
