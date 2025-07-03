import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import { fetchToken, updateToken } from "api/token";
import TokenForm from "components/TokenForm";
import { useNotification } from "context/notificationContext";
import { TokenFormValues } from "types/token";

const TokenConfiguration = () => {
  const { uuid } = useParams() as { uuid: string };
  const { notify } = useNotification();
  const navigate = useNavigate();

  const onSubmit = (values: TokenFormValues) => {
    updateToken(uuid, JSON.stringify(values, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Token ${uuid} updated`);
          navigate(`/ui/provisioning/tokens/${uuid}/configuration`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during token update: ${e}`);
      });
  };

  const {
    data: token = undefined,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["tokens", uuid],
    queryFn: () => fetchToken(uuid),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading tokens</div>;
  }

  return <TokenForm token={token} onSubmit={onSubmit} />;
};

export default TokenConfiguration;
