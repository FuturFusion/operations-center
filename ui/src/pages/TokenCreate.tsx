import { useNavigate } from "react-router";
import { useNotification } from "context/notificationContext";
import { createToken } from "api/token";
import TokenForm from "components/TokenForm";
import { TokenFormValues } from "types/token";

const TokenCreate = () => {
  const { notify } = useNotification();
  const navigate = useNavigate();

  const onSubmit = (values: TokenFormValues) => {
    createToken(JSON.stringify(values, null, 2))
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Token created`);
          navigate("/ui/provisioning/servers-view/tokens");
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during token creation: ${e}`);
      });
  };

  return (
    <div className="d-flex flex-column">
      <div className="scroll-container flex-grow-1 p-3">
        <TokenForm onSubmit={onSubmit} />
      </div>
    </div>
  );
};

export default TokenCreate;
