import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import { preRegisterServer } from "api/server";
import Breadcrumbs from "components/Breadcrumbs";
import ServerPreRegisterForm from "components/ServerPreRegisterForm";
import { useNotification } from "context/notificationContext";
import { ServerPreRegisterFormValues } from "types/server";

const ServerPreRegister = () => {
  const { notify } = useNotification();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const onSubmit = async (values: ServerPreRegisterFormValues) => {
    return preRegisterServer(
      JSON.stringify(
        {
          name: values.name,
          description: values.description,
          properties: values.properties,
          public_connection_url: values.public_connection_url,
          channel: values.channel,
          bmc_config: {
            // Only one API type is supported for now.
            api_type: values.bmc_endpoint ? "redfish-v1-generic" : "",
            endpoint: values.bmc_endpoint,
            certificate: values.bmc_certificate,
            auto_pin_certificate: values.bmc_auto_pin_certificate,
            username: values.bmc_username,
            password: values.bmc_password,
          },
        },
        null,
        2,
      ),
    )
      .then((response) => {
        if (response.error_code == 0) {
          notify.success(`Server ${values.name} pre registered`);
          queryClient.invalidateQueries({ queryKey: ["servers"] });
          navigate("/ui/provisioning/servers-view");
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        notify.error(`Error during server pre registration: ${e}`);
      });
  };

  return (
    <div className="d-flex flex-column">
      <Breadcrumbs />
      <div className="scroll-container flex-grow-1 p-3">
        <ServerPreRegisterForm onSubmit={onSubmit} />
      </div>
    </div>
  );
};

export default ServerPreRegister;
