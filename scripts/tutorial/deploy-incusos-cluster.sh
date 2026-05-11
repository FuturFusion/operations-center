#!/bin/bash

OIDC_CLIENT_ID=${OIDC_CLIENT_ID:-""}

INSTANCE_CPUS="${INSTANCE_CPUS:-4}"
INSTANCE_MEMORY="${INSTANCE_MEMORY:-4GiB}"
INSTANCE_DISK_SIZE="${INSTANCE_DISK_SIZE:-50GiB}"
INSTANCE_COUNT="${INSTANCE_COUNT:-3}"

echo "====[ Creating Incus project & profile ]===="
incus project create tutorial-incusos-cluster
incus project switch tutorial-incusos-cluster

incus profile device add default eth0 nic network=incusbr0 name=eth0
incus profile device add default root disk pool=default path=/

echo "====[ Getting seeded OperationsCenter ISO image ]===="
OPERATIONS_CENTER_SEED_FILE=$(mktemp --suffix=operation_center_seed_file.json)
CLIENT_CERT=$(incus remote get-client-certificate | jq -Rs .)

jq -n --arg client_cert "${CLIENT_CERT}" --arg oidc_client_id "${OIDC_CLIENT_ID}" '
{
  seeds: {
    install: {
      version: "1",
      force_install: false,
      force_reboot: false
    },
    applications: {
      version: "1",
      applications: [
        {
          name: "operations-center"
        }
      ]
    },
    "operations-center": (
      {
        version: "1",
        trusted_client_certificates: [
          ($client_cert | fromjson)
        ]
      }
      +
      (
        if $oidc_client_id != "" then
          {
            preseed: {
              system_security: {
                oidc: {
                  claim: "preferred_username",
                  client_id: $oidc_client_id,
                  issuer: "https://sso.linuxcontainers.org",
                  scopes: "openid,offline_access"
                }
              }
            }
          }
        else
          {}
        end
      )
    )
  },
  type: "iso",
  architecture: "x86_64"
}
' > "${OPERATIONS_CENTER_SEED_FILE}"

IMAGES_RESP=$(curl -s -X POST -d @${OPERATIONS_CENTER_SEED_FILE} "https://incusos-customizer.linuxcontainers.org/1.0/images")
DOWNLOAD_PATH=$(echo ${IMAGES_RESP} | jq -r '.metadata.image')
if [ ! -e ~/Downloads/IncusOS_OperationsCenter.iso ]; then
  curl -o ~/Downloads/IncusOS_OperationsCenter.iso --compressed "https://incusos-customizer.linuxcontainers.org${DOWNLOAD_PATH}"
fi

incus storage volume import default ~/Downloads/IncusOS_OperationsCenter.iso IncusOS_OperationsCenter.iso --type=iso

echo "====[ Setting up OperationsCenter VM ]===="
incus init --empty --vm OperationsCenter \
  -c security.secureboot=false \
  -c limits.cpu=${INSTANCE_CPUS} \
  -c limits.memory=${INSTANCE_MEMORY} \
  -d root,size=${INSTANCE_DISK_SIZE}
incus config device add OperationsCenter vtpm tpm
incus config device add OperationsCenter boot-media disk pool=default source=IncusOS_OperationsCenter.iso boot.priority=10
incus config set OperationsCenter systemd.credential.fully-enable-incus-agent=true

incus start OperationsCenter

# Wait for the VM to boot and complete installation
echo "====[ Waiting for OperationsCenter to complete installation ]===="
incus wait OperationsCenter agent
while ! incus exec OperationsCenter -- bash -c "journalctl -b -u incus-osd | grep -q 'IncusOS was successfully installed'"; do echo -n "."; sleep 1; done; echo ""

# Remove the boot media
echo "====[ Removing boot media from OperationsCenter VM and restart ]===="
incus stop OperationsCenter
incus config device remove OperationsCenter boot-media
incus start OperationsCenter

# Wait for the VM to be ready again
echo "====[ Waiting for OperationsCenter to be ready ]===="
incus wait OperationsCenter agent
while ! incus exec OperationsCenter -- bash -c "journalctl -b -u incus-osd | grep -q 'System is ready'"; do echo -n "."; sleep 1; done; echo ""

mkdir -p ~/.config/operations-center
cp ~/.config/incus/client.* ~/.config/operations-center/
OPERATIONS_CENTER_IP=$(incus list -f json | jq -r '.[] | select(.name == "OperationsCenter") | .state.network | to_entries[] | .value.addresses[]? | select(.family == "inet" and .scope == "global") | .address' | head -n1)
operations-center remote add tutorial-operations-center https://${OPERATIONS_CENTER_IP}:8443 --auth-type tls --accept-certificate
operations-center remote switch tutorial-operations-center

# Wait for the operations-center to self-register
echo "====[ Waiting for OperationsCenter to self-register ]===="
while ! operations-center provisioning server list -f json | jq -r -e '[ .[] | select(.name == "operations-center") ] | length == 1' > /dev/null; do echo -n "."; sleep 1; done; echo ""

# Wait for operations-center to have updates available
echo "====[ Waiting for OperationsCenter to have updates available ]===="
while ! operations-center provisioning update list -f json | jq -r -e '[ .[] | select(.update_status == "ready") ] | length >= 2' > /dev/null; do echo -n "."; sleep 1; done; echo ""

# Prepare for IncusOS Incus instances
operations-center provisioning token add --description "IncusOS Cluster Tutorial" --uses 20
INSTALLATION_TOKEN=$(operations-center provisioning token list -f json | jq -r '[ .[] | select(.description == "IncusOS Cluster Tutorial") ] | first | .uuid')

INCUS_PRE_SEED_FILE=$(mktemp --suffix=incus_pre_seed.yaml)
cat << EOF > "${INCUS_PRE_SEED_FILE}"
applications:
  version: "1"
  applications:
    - name: incus
    - name: debug
network:
  version: "1"
  interfaces:
    - name: enp5s0
      hwaddr: enp5s0
      required_for_online: both
      addresses:
      - dhcp4
      - dhcp6
      - slaac
incus:
  version: "1"
EOF

if [ -n "$OIDC_CLIENT_ID" ]; then
  cat << EOF >> "${INCUS_PRE_SEED_FILE}"
  preseed:
    config:
      core.https_address: ":8443"
      oidc.claim: "preferred_username"
      oidc.client.id: "${OIDC_CLIENT_ID}"
      oidc.issuer: "https://sso.linuxcontainers.org"
      oidc.scopes: "openid,offline_access"
EOF
fi

if [ -e ~/Downloads/IncusOS.iso ]; then
  rm -f ~/Downloads/IncusOS.iso
fi

echo "====[ Getting IncusOS ISO image from OperationsCenter ]===="
operations-center provisioning token get-image ${INSTALLATION_TOKEN} ~/Downloads/IncusOS.iso ${INCUS_PRE_SEED_FILE} --architecture x86_64 --type iso

incus storage volume import default ~/Downloads/IncusOS.iso IncusOS.iso --type=iso

SERVER_NAMES_ARGS=()

for i in $(seq 1 ${INSTANCE_COUNT}); do
  INSTANCE_NAME=$(printf "IncusOS%02d" "$i")
  echo "====[ Setting up IncusOS Instance ${INSTANCE_NAME} ]===="

  SERVER_NAMES_ARGS+=(--server-names "${INSTANCE_NAME}")

  incus init --empty --vm ${INSTANCE_NAME} \
    -c security.secureboot=false \
    -c limits.cpu=${INSTANCE_CPUS} \
    -c limits.memory=${INSTANCE_MEMORY} \
    -d root,size=${INSTANCE_DISK_SIZE}
  incus config device add ${INSTANCE_NAME} vtpm tpm
  incus config device add ${INSTANCE_NAME} boot-media disk pool=default source=IncusOS.iso boot.priority=10
  incus config set ${INSTANCE_NAME} systemd.credential.fully-enable-incus-agent=true

  incus start ${INSTANCE_NAME}

  # Wait for the VM to boot and complete installation
  echo "====[ Waiting for ${INSTANCE_NAME} to complete installation ]===="
  incus wait ${INSTANCE_NAME} agent
  while ! incus exec ${INSTANCE_NAME} -- bash -c "journalctl -b -u incus-osd | grep -q 'IncusOS was successfully installed'"; do echo -n "."; sleep 1; done; echo ""

  # Remove the boot media
  echo "====[ Removing boot media from ${INSTANCE_NAME} VM and restart ]===="
  incus stop ${INSTANCE_NAME}
  incus config device remove ${INSTANCE_NAME} boot-media
  incus start ${INSTANCE_NAME}

  echo "====[ Waiting for ${INSTANCE_NAME} to be ready ]===="
  incus wait ${INSTANCE_NAME} agent
  while ! incus exec ${INSTANCE_NAME} -- bash -c "journalctl -b -u incus-osd | grep -q 'System is ready'"; do echo -n "."; sleep 1; done; echo ""

  ## Rename IncusOS Servers
  echo "====[ Renaming ${INSTANCE_NAME} in OperationsCenter ]===="
  INSTANCE_ID=$(operations-center provisioning server list -f json | jq -r '.[] | select(.name != "operations-center" and (.name | test("^IncusOS[0-9]+$") | not ) ) | .name')
  operations-center provisioning server rename ${INSTANCE_ID} ${INSTANCE_NAME}
done

## Cluster the Servers

APPLICATION_CONFIG=$(mktemp --suffix=application_config.yaml)
cat << EOF > "${APPLICATION_CONFIG}"
certificates:
  - type: client
    name: my-client-cert
    description: "Client certificate for accessing the cluster"
    certificate: $(incus remote get-client-certificate | jq -Rs .)
config:
  core.https_address: ":8443"
EOF

if [ -n "$OIDC_CLIENT_ID" ]; then
  cat << EOF >> "${APPLICATION_CONFIG}"
  oidc.claim: "preferred_username"
  oidc.client.id: "${OIDC_CLIENT_ID}"
  oidc.issuer: "https://sso.linuxcontainers.org"
  oidc.scopes: "openid,offline_access"
EOF
fi

# wait for all servers to be ready
echo "====[ Waiting for all IncusOS servers to be ready in OperationsCenter ]===="
while ! operations-center provisioning server list -f json | jq -r -e '[ .[] | select((.name | test("^IncusOS[0-9]+$")) and (.server_status == "ready")) ] | length == 3' > /dev/null; do echo -n "."; sleep 1; done; echo ""

CONNECTION_URL_IP=$(incus list -f json | jq -r '.[] | select(.name == "IncusOS01") | .state.network | to_entries[] | .value.addresses[]? | select(.family == "inet" and .scope == "global") | .address' | head -n1)
# create cluster
echo "====[ Creating IncusOS cluster ]===="
operations-center provisioning cluster add tutorial-incusos-cluster https://${CONNECTION_URL_IP}:8443 "${SERVER_NAMES_ARGS[@]}" --application-seed-config ${APPLICATION_CONFIG}

## Access the IncusOS Cluster
echo "====[ Add remote ]===="
incus remote add tutorial-incusos-cluster https://${CONNECTION_URL_IP}:8443 --auth-type tls --accept-certificate
incus remote switch tutorial-incusos-cluster

echo "Operations Center URL: https://${OPERATIONS_CENTER_IP}:8443"
echo "IncusOS Cluster API Endpoint: https://${CONNECTION_URL_IP}:8443"
