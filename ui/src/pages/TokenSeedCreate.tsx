import { useNavigate, useParams } from "react-router";
import { useNotification } from "context/notificationContext";
import { createTokenSeed } from "api/token";
import TokenSeedForm from "components/TokenSeedForm";
import { TokenSeed } from "types/token";

const TokenSeedCreate = () => {
  const { notify } = useNotification();
  const navigate = useNavigate();
  const { uuid } = useParams<{ uuid: string }>();

  const onSubmit = (tokenSeed: TokenSeed) => {
    createTokenSeed(uuid || "", JSON.stringify(tokenSeed, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Token seed created`);
          navigate(`/ui/provisioning/tokens/${uuid}/seeds`);
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during token seed creation: ${e}`);
      });
  };

  return (
    <div className="d-flex flex-column">
      <div className="scroll-container flex-grow-1 p-3">
        <TokenSeedForm onSubmit={onSubmit} />
      </div>
    </div>
  );
};

export default TokenSeedCreate;
