import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import { fetchTokenSeed, updateTokenSeed } from "api/token";
import TokenSeedForm from "components/TokenSeedForm";
import { useNotification } from "context/notificationContext";
import { TokenSeed } from "types/token";

const TokenSeedConfiguration = () => {
  const { uuid, name } = useParams() as { uuid: string; name: string };
  const { notify } = useNotification();
  const navigate = useNavigate();

  const onSubmit = (tokenSeed: TokenSeed) => {
    updateTokenSeed(uuid, name, JSON.stringify(tokenSeed, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Token seed ${name} updated`);
          navigate(
            `/ui/provisioning/tokens/${uuid}/seeds/${name}/configuration`,
          );
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during token seed update: ${e}`);
      });
  };

  const {
    data: seed = undefined,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["tokens", uuid, "seeds", name],
    queryFn: () => fetchTokenSeed(uuid, name),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading token seed</div>;
  }

  return <TokenSeedForm seed={seed} onSubmit={onSubmit} />;
};

export default TokenSeedConfiguration;
