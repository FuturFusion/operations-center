import { FC, useState } from "react";
import { Form } from "react-bootstrap";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { MdStart } from "react-icons/md";
import { deployServer } from "api/server";
import { fetchTokens, fetchTokenSeeds } from "api/token";
import ArchSelect from "components/ArchSelect";
import ChannelSelect from "components/ChannelSelect";
import ImageTypeSelect from "components/ImageTypeSelect";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { Server } from "types/server";

interface Props {
  server: Server;
}

const ServerDeployBtn: FC<Props> = ({ server }) => {
  const [showModal, setShowModal] = useState(false);
  const [opInProgress, setOpInProgress] = useState(false);
  const [tokenUUID, setTokenUUID] = useState("");
  const [seed, setSeed] = useState("");
  const [virtualMediaID, setVirtualMediaID] = useState("");
  const [imageType, setImageType] = useState("iso");
  const [architecture, setArchitecture] = useState("");
  const [channel, setChannel] = useState("");
  const [skipSecureBootCertificates, setSkipSecureBootCertificates] =
    useState(false);
  const [force, setForce] = useState(false);
  const { notify } = useNotification();
  const queryClient = useQueryClient();

  const actionStyle = {
    cursor: "pointer",
    color: "grey",
  };

  const { data: tokens = [] } = useQuery({
    queryKey: ["tokens"],
    queryFn: fetchTokens,
    enabled: showModal,
  });

  const { data: seeds = [] } = useQuery({
    queryKey: ["tokens", tokenUUID, "seeds"],
    queryFn: () => fetchTokenSeeds(tokenUUID),
    enabled: showModal && tokenUUID != "",
  });

  const virtualMedia = Object.values(server.bmc_data?.virtual_media ?? {});

  const reset = () => {
    setTokenUUID("");
    setSeed("");
    setVirtualMediaID("");
    setImageType("iso");
    setArchitecture("");
    setChannel("");
    setSkipSecureBootCertificates(false);
    setForce(false);
  };

  const handleClose = () => {
    setShowModal(false);
    reset();
  };

  const onDeployServer = () => {
    setOpInProgress(true);
    deployServer(
      server.name,
      JSON.stringify(
        {
          token_uuid: tokenUUID,
          seed: seed,
          type: imageType,
          architecture: architecture,
          channel: channel,
          virtual_media_id: virtualMediaID,
          force: force,
          skip_secure_boot_certificates: skipSecureBootCertificates,
        },
        null,
        2,
      ),
    )
      .then(() => {
        setOpInProgress(false);
        notify.success(`Deployment of server "${server.name}" triggered`);
        queryClient.invalidateQueries({ queryKey: ["servers"] });
        handleClose();
      })
      .catch((e) => {
        setOpInProgress(false);
        notify.error(`Error during server deployment: ${e}`);
      });
  };

  return (
    <>
      <MdStart
        size={25}
        title="Deploy IncusOS"
        style={actionStyle}
        onClick={() => {
          setShowModal(true);
        }}
      />
      <ModalWindow
        show={showModal}
        scrollable
        handleClose={handleClose}
        title={`Deploy IncusOS on "${server.name}"`}
        footer={
          <LoadingButton
            isLoading={opInProgress}
            variant="success"
            disabled={tokenUUID == "" || seed == ""}
            onClick={onDeployServer}
          >
            Deploy
          </LoadingButton>
        }
      >
        <Form>
          <Form.Group className="mb-3" controlId="token_uuid">
            <Form.Label>Token</Form.Label>
            <Form.Select
              value={tokenUUID}
              onChange={(e) => {
                setTokenUUID(e.target.value);
                setSeed("");
              }}
              disabled={opInProgress}
            >
              <option key="" value=""></option>
              {tokens.map((token) => (
                <option key={token.uuid} value={token.uuid}>
                  {token.uuid}
                  {token.description && ` - ${token.description}`} (
                  {token.uses_remaining} uses left)
                </option>
              ))}
            </Form.Select>
          </Form.Group>
          <Form.Group className="mb-3" controlId="seed">
            <Form.Label>Token seed</Form.Label>
            <Form.Select
              value={seed}
              onChange={(e) => setSeed(e.target.value)}
              disabled={opInProgress || tokenUUID == ""}
            >
              <option key="" value=""></option>
              {seeds.map((tokenSeed) => (
                <option
                  key={tokenSeed.name}
                  value={tokenSeed.name}
                  disabled={!tokenSeed.public}
                >
                  {tokenSeed.name}
                  {tokenSeed.description && ` - ${tokenSeed.description}`}
                  {!tokenSeed.public && " (not public)"}
                </option>
              ))}
            </Form.Select>
            <Form.Text>
              The BMC fetches the installation media without authentication, so
              only public seeds can be deployed.
            </Form.Text>
          </Form.Group>
          <Form.Group className="mb-3" controlId="virtual_media_id">
            <Form.Label>Virtual media</Form.Label>
            <Form.Select
              value={virtualMediaID}
              onChange={(e) => setVirtualMediaID(e.target.value)}
              disabled={opInProgress}
            >
              <option key="" value="">
                Automatic
              </option>
              {virtualMedia.map((media) => (
                <option key={media.id} value={media.id}>
                  {media.id}
                  {media.media_types?.length > 0 &&
                    ` (${media.media_types.join(", ")})`}
                </option>
              ))}
            </Form.Select>
          </Form.Group>
          <ImageTypeSelect value={imageType} onChange={setImageType} />
          <ArchSelect value={architecture} onChange={setArchitecture} />
          <ChannelSelect
            value={channel}
            onChange={setChannel}
            disabled={opInProgress}
            formClasses="mb-3 mt-3"
          />
          <Form.Group
            className="mb-3"
            controlId="skip_secure_boot_certificates"
          >
            <Form.Check
              type="checkbox"
              label="Skip the enrollment of the secure boot certificates"
              checked={skipSecureBootCertificates}
              onChange={(e) => setSkipSecureBootCertificates(e.target.checked)}
              disabled={opInProgress}
            />
            <Form.Text>
              Required for a BMC, whose Redfish API does not support the
              modification of the UEFI key databases. The certificates then have
              to be enrolled manually before the deployment.
            </Form.Text>
          </Form.Group>
          <Form.Group className="mb-3" controlId="force">
            <Form.Check
              type="checkbox"
              label='Accept a token seed, that does not set "force_reboot"'
              checked={force}
              onChange={(e) => setForce(e.target.checked)}
              disabled={opInProgress}
            />
            <Form.Text>
              The deployment then relies on the read progress of the
              installation media alone to tell, when the first stage of the
              installation is done.
            </Form.Text>
          </Form.Group>
        </Form>
      </ModalWindow>
    </>
  );
};

export default ServerDeployBtn;
