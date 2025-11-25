ARG GO_VERSION=1.24
ARG DEBIAN_VERSION=trixie

# Go development container

FROM golang:${GO_VERSION}-${DEBIAN_VERSION}
ARG TARGETOS
ARG TARGETARCH
ARG USERNAME=vscode
ARG USER_UID=1000
ARG USER_GID=1000
ARG INCUS_TERRAFORM_PROVIDER_VERSION=1.0.0

# Install necessary tools.
RUN <<EOF
echo 'deb [trusted=yes] https://repo.goreleaser.com/apt/ /' | tee /etc/apt/sources.list.d/goreleaser.list
curl -fsSL https://get.opentofu.org/opentofu.gpg | tee /etc/apt/keyrings/opentofu.gpg >/dev/null
curl -fsSL https://packages.opentofu.org/opentofu/tofu/gpgkey | gpg --no-tty --batch --dearmor -o /etc/apt/keyrings/opentofu-repo.gpg >/dev/null

cat <<EOT | tee /etc/apt/sources.list.d/opentofu.list > /dev/null
deb [signed-by=/etc/apt/keyrings/opentofu.gpg,/etc/apt/keyrings/opentofu-repo.gpg] https://packages.opentofu.org/opentofu/tofu/any/ any main
deb-src [signed-by=/etc/apt/keyrings/opentofu.gpg,/etc/apt/keyrings/opentofu-repo.gpg] https://packages.opentofu.org/opentofu/tofu/any/ any main
EOT

apt update
apt install -y \
  aspell \
  aspell-en \
  direnv \
  docker.io \
  fish \
  goreleaser \
  jq \
  kpartx \
  less \
  locales \
  locales-all \
  man \
  python3.13-venv \
  ruby-mdl \
  sqlite3 \
  sudo \
  tofu \
  unzip \
  vim \
  zsh

mkdir -p /usr/local/share/terraform/plugins/registry.opentofu.org/lxc/incus/${INCUS_TERRAFORM_PROVIDER_VERSION}/${TARGETOS}_${TARGETARCH}
curl -sLo /usr/local/share/terraform/plugins/terraform-provider-incus.zip https://github.com/lxc/terraform-provider-incus/releases/download/v${INCUS_TERRAFORM_PROVIDER_VERSION}/terraform-provider-incus_${INCUS_TERRAFORM_PROVIDER_VERSION}_${TARGETOS}_${TARGETARCH}.zip
unzip -d /usr/local/share/terraform/plugins/registry.opentofu.org/lxc/incus/${INCUS_TERRAFORM_PROVIDER_VERSION}/${TARGETOS}_${TARGETARCH} /usr/local/share/terraform/plugins/terraform-provider-incus.zip
EOF

# Install tools for building UI.
RUN curl -sL https://deb.nodesource.com/setup_22.x | sudo -E bash - && \
    apt update && \
    apt install nodejs && \
    npm install --global yarn

# Add vscode user and add it to sudoers.
RUN groupadd -g 1000 $USERNAME && \
    useradd -s /bin/bash -u $USER_UID -g $USER_GID -m $USERNAME && \
    usermod -aG docker $USERNAME && \
    mkdir -p /etc/sudoers.d && \
    echo $USERNAME ALL=\(root\) NOPASSWD:ALL > /etc/sudoers.d/$USERNAME && \
    chmod 0440 /etc/sudoers.d/$USERNAME

# Make sure we have a loop back device.
RUN test -b /dev/loop0 || mknod -m 660 /dev/loop0 b 7 0

# Setup for vscode user.
USER $USERNAME
ENV EDITOR=vi \
    LANG=en_US.UTF-8 \
    LC_ALL=en_US.UTF-8
# Build Go tools with user vscode to ensure correct file and directory permissions for the build artifacts.
RUN go install -v github.com/google/go-licenses@latest && \
    go install -v github.com/766b/go-outliner@latest && \
    GOTOOLCHAIN="" go install -v golang.org/x/tools/gopls@latest && \
    go install -v github.com/go-delve/delve/cmd/dlv@latest && \
    go install -v golang.org/x/tools/cmd/goimports@latest && \
    go install -v golang.org/x/vuln/cmd/govulncheck@latest && \
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
USER root

# Since we use a volume for /go to persist the content between executions, we need to preserve the binaries.
RUN mv /go/bin/* /usr/local/bin
