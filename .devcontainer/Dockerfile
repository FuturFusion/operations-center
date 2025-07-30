ARG GO_VERSION=1.23
ARG DEBIAN_VERSION=bookworm

# Go development container

FROM golang:${GO_VERSION}-${DEBIAN_VERSION}
ARG USERNAME=vscode
ARG USER_UID=1000
ARG USER_GID=1000

# Install necessary tools.
RUN echo 'deb [trusted=yes] https://repo.goreleaser.com/apt/ /' | tee /etc/apt/sources.list.d/goreleaser.list && \
    apt update && \
    apt install -y vim sudo man less goreleaser jq fish zsh sqlite3 direnv docker.io kpartx

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
    LANG=en_US.UTF-8
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
