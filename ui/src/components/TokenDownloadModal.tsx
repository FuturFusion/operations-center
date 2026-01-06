import { FC } from "react";
import { Button } from "react-bootstrap";
import { FormikErrors, useFormik } from "formik";
import { downloadImage } from "api/token";
import TokenImageForm from "components/TokenImageForm";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { Token } from "types/token";
import { TokenImageFormValues } from "types/token";
import { BootSecurity } from "util/token";
import { downloadFile } from "util/util";
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
      secondary_applications: [],
      migration_manager: "",
      operations_center: "",
      install: {
        force_install: true,
        force_reboot: false,
        boot_security: BootSecurity.OPTIMAL,
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

      let applications = [{ name: values.seeds.application }];
      if (values.seeds.application == "incus") {
        applications = [
          ...applications,
          ...values.seeds.secondary_applications.map((app) => {
            return { name: app };
          }),
        ];
      }

      handleClose();

      const bootSecurity = values.seeds.install.boot_security;

      delete values.seeds.install.boot_security;

      download({
        ...values,
        seeds: {
          applications: { applications: applications },
          install: {
            ...values.seeds.install,
            security: {
              missing_tpm: bootSecurity == BootSecurity.NO_TPM,
              missing_secure_boot: bootSecurity == BootSecurity.NO_SECURE_BOOT,
            },
          },
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
      const filename = `${token.uuid}.${(values as TokenImageFormValues).type}`;

      downloadFile(url, filename);
    } catch (error) {
      notify.error(`Error during image downloading: ${error}`);
    }

    downloadChanged(false);
  };

  return (
    <ModalWindow
      show={show}
      scrollable
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
