import { useQuery, useQueryClient } from "@tanstack/react-query";
import { fetchSystemCertificate, updateSystemCertificate } from "api/settings";
import SystemCertForm from "components/SystemCertForm";
import SystemCertOverview from "components/SystemCertOverview";
import { useNotification } from "context/notificationContext";
import { SystemCertificatePost } from "types/settings";

const SystemCertConfiguration = () => {
  const { notify } = useNotification();
  const queryClient = useQueryClient();

  const onSubmit = (certificate: SystemCertificatePost) => {
    updateSystemCertificate(JSON.stringify(certificate, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`System certificate updated`);
          queryClient.invalidateQueries({ queryKey: ["system_certificate"] });
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during system certificate update: ${e}`);
      });
  };

  const {
    data: certificate = undefined,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["system_certificate"],
    queryFn: () => fetchSystemCertificate(),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading system certificate</div>;
  }

  return (
    <>
      <SystemCertOverview certificate={certificate} />
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
