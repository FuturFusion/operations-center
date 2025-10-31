import { updateSystemCertificate } from "api/settings";
import SystemCertForm from "components/SystemCertForm";
import { useNotification } from "context/notificationContext";
import { SystemCertificate } from "types/settings";

const SystemCertConfiguration = () => {
  const { notify } = useNotification();

  const onSubmit = (certificate: SystemCertificate) => {
    updateSystemCertificate(JSON.stringify(certificate, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`System certificate updated`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during system certificate update: ${e}`);
      });
  };

  return <SystemCertForm onSubmit={onSubmit} />;
};

export default SystemCertConfiguration;
