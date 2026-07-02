#!/bin/bash
# Build script for coi image for Go development on Debian 13 and Claude CLI.
# This script runs INSIDE the container during image build.
#
# Originally based on (MIT License):
# https://github.com/mensfeld/code-on-incus/blob/2f15f5e6abe2d4a5d48e976b1937522717d82a79/internal/image/build.sh
#
# It installs all dependencies needed for CLI tool execution:
# - Base development tools
# - Node.js LTS (system, for Claude CLI)
# - Go + tools (gopls, delve, goimports, govulncheck, lego, go-licenses, go-outliner, golangci-lint)
# - Claude CLI
# - Docker
# - GitHub CLI

set -euo pipefail

# Configuration
CODE_USER="code"
CODE_UID=1000

GO_VERSION=1.25.11
INCUS_TERRAFORM_PROVIDER_VERSION=1.1.0

log() {
    echo "[coi] $*"
}

#######################################
# Configure DNS if misconfigured
# Only applies fix if DNS resolution fails
#######################################
configure_dns_if_needed() {
    log "Checking DNS configuration..."

    # Test if DNS resolution works
    if getent hosts archive.ubuntu.com > /dev/null 2>&1; then
        log "DNS resolution works, keeping default configuration."
        return 0
    fi

    log "DNS resolution failed, configuring static DNS..."

    # Disable systemd-resolved (not needed in containers)
    systemctl disable systemd-resolved 2>/dev/null || true
    systemctl stop systemd-resolved 2>/dev/null || true
    systemctl mask systemd-resolved 2>/dev/null || true

    # Remove symlink and create static resolv.conf
    rm -f /etc/resolv.conf
    cat > /etc/resolv.conf << 'EOF'
# Static DNS configuration (auto-configured due to DNS misconfiguration)
# See: https://github.com/mensfeld/code-on-incus#troubleshooting
nameserver 8.8.8.8
nameserver 8.8.4.4
nameserver 1.1.1.1
EOF

    log "Static DNS configured (8.8.8.8, 8.8.4.4, 1.1.1.1)."

    # Verify it works now
    if getent hosts archive.ubuntu.com > /dev/null 2>&1; then
        log "DNS resolution now working."
    else
        log "WARNING: DNS still not working after fix. Build may fail."
    fi
}

#######################################
# Install base dependencies
#######################################
install_base_dependencies() {
    log "Installing base dependencies..."

    apt-get update -qq

    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
        aspell \
        aspell-en \
        bat \
        build-essential \
        ca-certificates \
        curl \
        direnv \
        dnsutils \
        fzf \
        fd-find \
        gnupg \
        jq \
        kpartx \
        less \
        locales \
        locales-all \
        lsof \
        man-db \
        pip \
        python3.13-venv \
        ripgrep \
        ruby-mdl \
        sqlite3 \
        strace \
        sudo \
        tmux \
        tree \
        unzip \
        vim \
        wget

    # On Debian, fd-find and bat install as fdfind and batcat.
    # Create symlinks so the standard names work.
    ln -sf "$(command -v fdfind)" /usr/local/bin/fd
    ln -sf "$(command -v batcat)" /usr/local/bin/bat

    log "Base dependencies installed"
}

#######################################
# Install Go
#######################################
install_go() {
    log "Installing Go ${GO_VERSION}..."

    wget -q "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -O "go${GO_VERSION}.linux-amd64.tar.gz"
    rm -rf /usr/local/go && tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz

    log "Go $(/usr/local/go/bin/go version) installed"
}

#######################################
# Install Node.js LTS
#######################################
install_nodejs() {
    log "Installing Node.js LTS..."

    curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
    apt-get install -y -qq nodejs

    log "Node.js $(node --version) installed"
}

#######################################
# Install goreleaser
#######################################
install_goreleaser() {
    log "Installing goreleaser..."

    echo 'deb [trusted=yes] https://repo.goreleaser.com/apt/ /' | tee /etc/apt/sources.list.d/goreleaser.list
    curl -fsSL https://get.opentofu.org/opentofu.gpg | tee /etc/apt/keyrings/opentofu.gpg >/dev/null
    curl -fsSL https://packages.opentofu.org/opentofu/tofu/gpgkey | gpg --no-tty --batch --dearmor -o /etc/apt/keyrings/opentofu-repo.gpg >/dev/null

    apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq goreleaser

    log "goreleaser installed"
}

#######################################
# Install opentofu
#######################################
install_opentofu() {
    log "Installing opentofu..."

    cat <<EOT | tee /etc/apt/sources.list.d/opentofu.list > /dev/null
deb [signed-by=/etc/apt/keyrings/opentofu.gpg,/etc/apt/keyrings/opentofu-repo.gpg] https://packages.opentofu.org/opentofu/tofu/any/ any main
deb-src [signed-by=/etc/apt/keyrings/opentofu.gpg,/etc/apt/keyrings/opentofu-repo.gpg] https://packages.opentofu.org/opentofu/tofu/any/ any main
EOT

    apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq tofu

    mkdir -p /usr/local/share/terraform/plugins/registry.opentofu.org/lxc/incus/${INCUS_TERRAFORM_PROVIDER_VERSION}/linux_amd64
    curl -sLo /usr/local/share/terraform/plugins/terraform-provider-incus.zip https://github.com/lxc/terraform-provider-incus/releases/download/v${INCUS_TERRAFORM_PROVIDER_VERSION}/terraform-provider-incus_${INCUS_TERRAFORM_PROVIDER_VERSION}_linux_amd64.zip
    unzip -d /usr/local/share/terraform/plugins/registry.opentofu.org/lxc/incus/${INCUS_TERRAFORM_PROVIDER_VERSION}/linux_amd64 /usr/local/share/terraform/plugins/terraform-provider-incus.zip

    log "opentofu installed"
}

#######################################
# Create code user with passwordless sudo
#######################################
create_code_user() {
    log "Creating code user..."

    groupadd -g "$CODE_UID" "$CODE_USER" 2>/dev/null || true
    useradd -m -u "$CODE_UID" -g "$CODE_USER" -s /bin/bash "$CODE_USER"

    mkdir -p "/home/$CODE_USER/.claude"
    mkdir -p "/home/$CODE_USER/.ssh"
    chmod 700 "/home/$CODE_USER/.ssh"
    # Pre-populate known_hosts. Try ssh-keyscan first (fresh keys); fall back to
    # embedded static keys for hosts where the SSH endpoint is unreachable (e.g.
    # bitbucket.org blocks automated scans, may be in maintenance windows).
    for _host in github.com gitlab.com bitbucket.org; do
        for _attempt in 1 2 3; do
            if ssh-keyscan -T 10 -t ed25519,rsa,ecdsa "$_host" >> "/home/$CODE_USER/.ssh/known_hosts" 2>/dev/null; then
                break
            fi
            sleep 2
        done
    done
    unset _host _attempt

    # Ensure bitbucket.org is always present — its SSH endpoint is sometimes
    # unavailable (maintenance, IP rate-limiting). These are the published keys
    # from https://www.atlassian.com/blog/bitbucket/ssh-host-key-changes
    if ! grep -q "^bitbucket.org " "/home/$CODE_USER/.ssh/known_hosts" 2>/dev/null; then
        cat >> "/home/$CODE_USER/.ssh/known_hosts" <<'BBKEYS'
bitbucket.org ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBPIQmuzMBuKdWeF4+a2sjSSpBK0iqitSQ+5BM9KhpexuGt20JpTVM7u5BDZngncgrqDMbWdxMWWOGtZ9UgbqgZE=
bitbucket.org ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIIazEu89wgQZ4bqs3d63QSMzYVa0MuJ2e2gKTKqu+UUO
bitbucket.org ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQDQeJzhupRu0u0cdegZIa8e86EG2qOCsIsD1Xw0xSeiPDlCr7kq97NLmMbpKTX6Esc30NuoqEEHCuc7yWtwp8dI76EEEB1VqY9QJq6vk+aySyboD5QF61I/1WeTwu+deCbgKMGbUijeXhtfbxSxm6JwGrXrhBdofTsbKRUsrN1WoNgUa8uqN1Vx6WAJw1JHPhglEGGHea6QICwJOAr/6mrui/oB7pkaWKHj3z7d1IC4KWLtY47elvjbaTlkN04Kc/5LFEirorGYVbt15kAUlqGM65pk6ZBxtaO3+30LVlORZkxOh+LKL/BvbZ/iRNhItLqNyieoQj/uh/7Iv4uyH/cV/0b4WDSd3DptigWq84lJubb9t/DnZlrJazxyDCulTmKdOR7vs9gMTo+uoIrPSb8ScTtvw65+odKAlBj59dhnVp9zd7QUojOpXlL62Aw56U4oO+FALuevvMjiWeavKhJqlR7i5n9srYcrNV7ttmDw7kf/97P5zauIhxcjX+xHv4M=
BBKEYS
    fi
    chmod 644 "/home/$CODE_USER/.ssh/known_hosts"
    chown -R "$CODE_USER:$CODE_USER" "/home/$CODE_USER"

    # Setup passwordless sudo for all commands
    echo "$CODE_USER ALL=(ALL) NOPASSWD:ALL" > "/etc/sudoers.d/$CODE_USER"

    chown root:root "/etc/sudoers.d/$CODE_USER"
    chmod 440 "/etc/sudoers.d/$CODE_USER"
    usermod -aG sudo "$CODE_USER"

    echo "export HOME=\"/home/$CODE_USER\"" >> "/home/$CODE_USER/.bashrc"
    echo 'export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"' >> "/home/$CODE_USER/.bashrc"

    sed -i 's/^#alias /alias /' "/home/$CODE_USER/.bashrc"

    log "User '$CODE_USER' created with passwordless sudo (uid: $CODE_UID)"
}

#######################################
# Configure power management wrappers
#######################################
configure_power_wrappers() {
    log "Configuring power management command wrappers..."

    # Create wrapper scripts that use sudo automatically
    # This allows users to type "poweroff" instead of "sudo poweroff"
    # while working around the lack of login sessions in containers

    # Incus assigns the container hostname at boot (from the UTS namespace), but
    # /etc/hosts is baked into the image at build time with a different name.
    # sudo looks up the current hostname for logging; if it is not in /etc/hosts
    # the lookup fails and prints "unable to resolve host" on every invocation.
    #
    # Fix: a oneshot systemd service that appends the runtime hostname to
    # /etc/hosts before multi-user.target is reached (i.e. before any shell).
    cat > /etc/systemd/system/coi-fix-hostname.service << 'UNIT_EOF'
[Unit]
Description=Add container hostname to /etc/hosts
After=local-fs.target
Before=network.target

[Service]
Type=oneshot
ExecStart=/bin/bash -c 'h=$(hostname); grep -qF "$h" /etc/hosts || echo "127.0.0.1 $h" >> /etc/hosts'
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
UNIT_EOF
    systemctl enable coi-fix-hostname.service

    # Power-off wrappers.
    #
    # Containers systemd-logind can be mid-start when a shutdown
    # is requested, producing a D-Bus transaction conflict.  One --force flag
    # makes systemctl talk directly to systemd (bypassing logind) without
    # resorting to a hard reboot() syscall, which lets COI clean up the session.
    for cmd in poweroff halt; do
        cat > "/usr/local/bin/${cmd}" << 'WRAPPER_EOF'
#!/bin/bash
exec sudo systemctl --force poweroff
WRAPPER_EOF
        chmod 755 "/usr/local/bin/${cmd}"
    done

    cat > "/usr/local/bin/reboot" << 'WRAPPER_EOF'
#!/bin/bash
exec sudo systemctl --force reboot
WRAPPER_EOF
    chmod 755 "/usr/local/bin/reboot"

    cat > "/usr/local/bin/shutdown" << 'WRAPPER_EOF'
#!/bin/bash
exec sudo systemctl --force poweroff
WRAPPER_EOF
    chmod 755 "/usr/local/bin/shutdown"

    # Create "close" as an alias for poweroff
    # This provides a safe alternative that doesn't exist on the host machine,
    # preventing accidental host shutdowns when typed outside the container
    cat > "/usr/local/bin/close" << 'WRAPPER_EOF'
#!/bin/bash
exec sudo systemctl --force poweroff
WRAPPER_EOF
    chmod 755 "/usr/local/bin/close"

    log "Power management wrappers configured"
}

#######################################
# Install Claude CLI using native installer
# Note: npm installation is deprecated as of 2025
# See: https://code.claude.com/docs/en/setup
#######################################
install_claude_cli() {
    log "Installing Claude CLI (native)..."

    # Prefer IPv4 to work around broken IPv6 in containers and some networks.
    # The native installer (Bun/Node) resolves AAAA records first; when the
    # IPv6 path is non-functional the download either times out or returns 403.
    # See: https://github.com/anthropics/claude-code/issues/13498
    if ! grep -q '::ffff:0:0/96' /etc/gai.conf 2>/dev/null; then
        echo 'precedence ::ffff:0:0/96 100' >> /etc/gai.conf
        log "IPv4 preference set in /etc/gai.conf"
    fi

    # Run the native installer as the code user (with retries for transient network failures)
    local attempt
    for attempt in 1 2 3; do
        if su - "$CODE_USER" -c 'curl -4 -fsSL https://claude.ai/install.sh | bash'; then
            break
        fi
        if [ "$attempt" -eq 3 ]; then
            log "ERROR: Claude CLI installation failed after 3 attempts."
            exit 1
        fi
        log "Claude CLI install failed (attempt $attempt/3), retrying in 10s..."
        sleep 10
    done

    # Verify that the installer actually created the Claude CLI binary
    local CLAUDE_PATH="/home/$CODE_USER/.local/bin/claude"
    if [[ ! -x "$CLAUDE_PATH" ]]; then
        log "ERROR: Claude CLI binary not found at $CLAUDE_PATH after installation."
        log "Installation may have failed or installed to an unexpected location."
        exit 1
    fi

    # Create a global symlink so it's accessible system-wide
    ln -sf "$CLAUDE_PATH" /usr/local/bin/claude

    log "Claude CLI $(claude --version 2>/dev/null || echo 'installed')"
}

#######################################
# Install Docker CE
#######################################
install_docker() {
    log "Installing Docker..."

    # Add Docker GPG key
    install -m 0755 -d /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/debian/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    chmod a+r /etc/apt/keyrings/docker.gpg

    # Add Docker repository
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian $(. /etc/os-release && echo $VERSION_CODENAME) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null

    # Install Docker
    apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
        docker-ce docker-ce-cli containerd.io \
        docker-buildx-plugin docker-compose-plugin

    # Add code user to docker group (belt-and-suspenders, may not be sufficient
    # on its own — see daemon config below for the reliable fix)
    usermod -aG docker "$CODE_USER"

    # Make the Docker socket accessible to the code user's PRIMARY group.
    #
    # Why: incus exec may not call initgroups() when --group is specified,
    # so supplementary groups (including 'docker') may not be active in the
    # session. The user's primary group (code, GID 1000) is always active
    # regardless of how the session was started.
    #
    # Two layers of config are needed:
    #
    # 1. daemon.json "group": "code" — Docker daemon chowns the socket to
    #    root:code on startup (works when Docker creates the socket itself).
    #
    # 2. docker.socket systemd drop-in SocketGroup=code — Docker
    #    package uses systemd socket activation: systemd creates
    #    /var/run/docker.sock before the daemon starts using the group from
    #    the socket unit (default: docker). The daemon.json setting alone is
    #    not enough when systemd socket activation is in play; this drop-in
    #    ensures the socket is created with root:code (0660) from the start.
    mkdir -p /etc/docker
    cat > /etc/docker/daemon.json << 'EOF'
{
    "group": "code"
}
EOF

    mkdir -p /etc/systemd/system/docker.socket.d
    cat > /etc/systemd/system/docker.socket.d/override.conf << 'EOF'
[Socket]
SocketGroup=code
EOF

    log "Docker $(docker --version 2>/dev/null || echo 'installed')"
}

#######################################
# Install GitHub CLI
#######################################
install_github_cli() {
    log "Installing GitHub CLI..."

    # Add GitHub CLI GPG key
    curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg
    chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg

    # Add GitHub CLI repository
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null

    # Install
    apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq gh

    log "GitHub CLI $(gh --version 2>/dev/null | head -1 || echo 'installed')"
}

#######################################
# Install Go tools
#######################################
install_go_tools() {
    log "Installing Go tools..."

    # Install Go tools as the code user (to avoid permission issues)
    su - "$CODE_USER" -c '/usr/local/go/bin/go install -v github.com/google/go-licenses@latest'
    su - "$CODE_USER" -c '/usr/local/go/bin/go install -v github.com/766b/go-outliner@latest'
    su - "$CODE_USER" -c 'GOTOOLCHAIN="" /usr/local/go/bin/go install -v golang.org/x/tools/gopls@latest'
    su - "$CODE_USER" -c '/usr/local/go/bin/go install -v github.com/go-delve/delve/cmd/dlv@latest'
    su - "$CODE_USER" -c '/usr/local/go/bin/go install -v golang.org/x/tools/cmd/goimports@latest'
    su - "$CODE_USER" -c '/usr/local/go/bin/go install -v golang.org/x/vuln/cmd/govulncheck@latest'
    su - "$CODE_USER" -c '/usr/local/go/bin/go install -v github.com/go-acme/lego/v4/cmd/lego@latest'
    su - "$CODE_USER" -c 'curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(/usr/local/go/bin/go env GOPATH)/bin'

    mv /home/$CODE_USER/go/bin/* /usr/local/bin

    log "Go tools installed"
}

#######################################
# Configure /tmp auto-cleanup
#
# By default systemd-tmpfiles-clean.timer runs daily and only
# removes files older than 10 days. AI coding agents can fill /tmp in
# minutes, so we:
#   1. Lower the age threshold to 1 hour so abandoned temp files are
#      collected promptly.
#   2. Run the cleanup timer every 15 minutes instead of daily so recovery
#      happens automatically between heavy operations.
#
# This complements the hard tmpfs size cap set by COI at container start.
#######################################
configure_tmp_cleanup() {
    log "Configuring /tmp auto-cleanup..."

    # Age threshold: remove files in /tmp not accessed for more than 1 hour.
    # The 'D' type removes the directory contents but keeps /tmp itself.
    cat > /etc/tmpfiles.d/coi-tmp-cleanup.conf << 'EOF'
# COI: clean files in /tmp that have not been accessed for 1 hour.
# This prevents abandoned build artefacts from exhausting the tmpfs.
D /tmp 1777 root root 1h
EOF

    # Override the cleanup timer to run every 15 minutes.
    mkdir -p /etc/systemd/system/systemd-tmpfiles-clean.timer.d
    cat > /etc/systemd/system/systemd-tmpfiles-clean.timer.d/coi-interval.conf << 'EOF'
[Timer]
# Reset inherited values before setting our own
OnBootSec=
OnUnitActiveSec=
# Start 5 minutes after boot, then every 15 minutes
OnBootSec=5min
OnUnitActiveSec=15min
EOF

    # Enable the timer (it is masked in some minimal images)
    systemctl enable systemd-tmpfiles-clean.timer 2>/dev/null || true

    log "/tmp cleanup configured (1h age threshold, 15min timer)"
}

#######################################
# Configure tmux scrollback history
#
# `coi shell` runs the interactive session inside a tmux session so
# that an exit of the inner CLI (e.g. Ctrl-C out of opencode) does not
# tear down the shell. Tmux's default `history-limit` is 2000 lines,
# which silently truncates the beginning of long command outputs
# (e.g. `bin/setup` in a Rails app) so the user cannot scroll back to
# the start. Bump the default high enough to cover realistic build
# logs. Users who want a different value can override this by writing
# their own `~/.tmux.conf` — tmux loads it after `/etc/tmux.conf` so
# per-user settings always win.
#######################################
configure_tmux() {
    log "Configuring tmux..."

    cat > /etc/tmux.conf << 'EOF'
# COI default: large scrollback so long build outputs (bin/setup, npm ci,
# cargo build, etc.) are fully retrievable. Override in ~/.tmux.conf —
# tmux loads the per-user file after this one, so your value wins.
set -g history-limit 50000

# Reduce escape-time from the default 500ms to 10ms so that the Escape key
# is delivered promptly to applications (e.g. vim, opencode) even when the
# container tmux is nested inside one or more outer tmux sessions on the
# host. With the default, each nesting layer adds up to 500ms of latency,
# making Esc feel completely broken. 10ms is enough to distinguish Esc from
# the start of an escape sequence without noticeable delay.
set -g escape-time 10
EOF
    chmod 644 /etc/tmux.conf

    log "tmux configured: history-limit=50000, escape-time=10ms in /etc/tmux.conf"
}

#######################################
# Cleanup
#######################################
cleanup() {
    log "Cleaning up..."
    apt-get clean
    rm -rf /var/lib/apt/lists/*
    log "Cleanup complete"
}

#######################################
# Main
#######################################
main() {
    log "Starting coi image build..."

    configure_dns_if_needed
    install_base_dependencies
    install_nodejs
    install_go
    install_goreleaser
    install_opentofu
    create_code_user
    configure_power_wrappers
    configure_tmp_cleanup
    configure_tmux
    install_claude_cli
    install_docker
    install_github_cli
    install_go_tools
    cleanup

    log "coi image build complete!"
}

main "$@"
