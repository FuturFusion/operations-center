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

  return (
    <>
      <div className="form-container">
        <h6>
          By default Operations Center uses an automatically generated
          self-signed TLS certificate. To replace it with a valid certificate,
          please provide a replacement PEM-encoded X509 certificate and key.
        </h6>
      </div>
      <SystemCertForm onSubmit={onSubmit} />
    </>
  );
};

export default SystemCertConfiguration;
