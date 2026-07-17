#!/usr/bin/env bash
# oci-helper-go — one-line install script
# Usage: curl -fsSL https://raw.githubusercontent.com/viogus/oci-helper-go/main/install.sh | bash
# See --help for options.

set -euo pipefail

# ── Color helpers ────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

info()    { printf "${GREEN}[INFO]${NC}    %s\n" "$*"; }
warn()    { printf "${YELLOW}[WARN]${NC}    %s\n" "$*"; }
error()   { printf "${RED}[ERROR]${NC}   %s\n" "$*"; }
step()    { printf "${CYAN}==>${NC} %s\n" "$*"; }

# ── Defaults ─────────────────────────────────────────────────────────────────
REPO="viogus/oci-helper-go"
IMAGE="ghcr.io/viogus/oci-helper-go"
BINARY_NAME="oci-helper"
INSTALL_DIR="/usr/local/bin"
ENV_FILE="/etc/oci-helper/env"
SERVICE_FILE="/etc/systemd/system/oci-helper.service"
APP_DIR="/app/oci-helper"
KEYS_DIR="/app/oci-helper/keys"

MODE=""                # docker, bare (auto-detected)
PORT="8818"
USERNAME="admin"
PASSWORD=""
DB_PATH="/app/oci-helper/oci-helper.db"
KEYS_DIR_OVERRIDE="/app/oci-helper/keys"
VERSION="latest"
DO_UNINSTALL=false

# ── Usage ────────────────────────────────────────────────────────────────────
usage() {
    cat <<EOF
${BOLD}oci-helper-go install script${NC}

Usage: curl -fsSL https://raw.githubusercontent.com/viogus/oci-helper-go/main/install.sh | bash -s -- [FLAGS]

${BOLD}Flags:${NC}
  --docker              Force Docker-based installation
  --bare                Force bare-metal (binary) installation
  --port PORT           Server port (default: 8818)
  --username USER       Admin username (default: admin)
  --password PASS       Admin password (auto-generated if omitted)
  --db-path PATH        SQLite database path (default: /app/oci-helper/oci-helper.db)
  --keys-dir PATH       OCI keys directory (default: /app/oci-helper/keys)
  --version X.Y.Z       Install specific version (default: latest)
  --uninstall           Remove oci-helper completely
  --help                Show this help message

${BOLD}Examples:${NC}
  # Auto-detect mode (docker if available, else bare-metal)
  curl -fsSL https://.../install.sh | bash

  # Force bare-metal installation
  curl -fsSL https://.../install.sh | bash -s -- --bare

  # With flags
  curl -fsSL https://.../install.sh | bash -s -- --port 9090 --username myadmin

  # Uninstall
  curl -fsSL https://.../install.sh | bash -s -- --uninstall
EOF
}

# ── Argument parsing ─────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case "$1" in
        --docker)       MODE="docker"; shift ;;
        --bare)         MODE="bare"; shift ;;
        --port)         PORT="$2"; shift 2 ;;
        --username)     USERNAME="$2"; shift 2 ;;
        --password)     PASSWORD="$2"; shift 2 ;;
        --db-path)      DB_PATH="$2"; shift 2 ;;
        --keys-dir)     KEYS_DIR_OVERRIDE="$2"; shift 2 ;;
        --version)      VERSION="$2"; shift 2 ;;
        --uninstall)    DO_UNINSTALL=true; shift ;;
        --help)         usage; exit 0 ;;
        *)              error "Unknown flag: $1"; usage; exit 1 ;;
    esac
done

# ── Require root ─────────────────────────────────────────────────────────────
require_root() {
    if [[ $EUID -ne 0 ]]; then
        error "This script must be run as root (use sudo)."
        exit 1
    fi
}

# ── Detect OS/Arch ───────────────────────────────────────────────────────────
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$ARCH" in
        x86_64|amd64)  GOARCH="amd64" ;;
        aarch64|arm64) GOARCH="arm64" ;;
        *) error "Unsupported architecture: $ARCH"; exit 1 ;;
    esac

    case "$OS" in
        linux)  GOOS="linux" ;;
        darwin) GOOS="darwin" ;;
        *) error "Unsupported OS: $OS (only linux and darwin are supported)"; exit 1 ;;
    esac

    info "Detected platform: ${GOOS}/${GOARCH}"
}

# ── Auto-detect docker availability ──────────────────────────────────────────
detect_docker() {
    if command -v docker &>/dev/null && docker info &>/dev/null 2>&1; then
        return 0
    fi
    return 1
}

# ── Generate password ────────────────────────────────────────────────────────
generate_password() {
    if [[ -n "$PASSWORD" ]]; then
        return
    fi
    # Generate a 24-character alphanumeric password
    PASSWORD=$(LC_ALL=C tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 24)
}

# ── Check if already installed ───────────────────────────────────────────────
check_existing() {
    local installed=false

    # Check for binary
    if command -v "$BINARY_NAME" &>/dev/null; then
        installed=true
    fi

    # Check for docker container
    if command -v docker &>/dev/null && docker ps -a --format '{{.Names}}' 2>/dev/null | grep -qx 'oci-helper'; then
        installed=true
    fi

    if $installed; then
        warn "oci-helper appears to be already installed."
        printf "    Choose action: [R]einstall/upgrade  [C]ancel  "
        read -r answer
        case "${answer,,}" in
            r|reinstall) info "Proceeding with reinstall/upgrade..." ;;
            *) info "Cancelled."; exit 0 ;;
        esac
    fi
}

# ── Stop & disable existing service ──────────────────────────────────────────
stop_existing() {
    # Stop and disable systemd service if it exists
    if systemctl is-active --quiet oci-helper 2>/dev/null; then
        step "Stopping existing oci-helper service..."
        systemctl stop oci-helper || true
    fi
    if systemctl is-enabled --quiet oci-helper 2>/dev/null; then
        systemctl disable oci-helper || true
    fi

    # Stop and remove docker container if it exists
    if command -v docker &>/dev/null; then
        if docker ps -a --format '{{.Names}}' 2>/dev/null | grep -qx 'oci-helper'; then
            step "Removing existing docker container..."
            docker stop oci-helper 2>/dev/null || true
            docker rm oci-helper 2>/dev/null || true
        fi
    fi
}

# ── Install via Docker ──────────────────────────────────────────────────────
install_docker() {
    step "Installing via Docker..."

    # Create directories
    step "Creating directory structure..."
    mkdir -p "$APP_DIR" "$KEYS_DIR"

    # Pull image
    local image_tag="${IMAGE}:${VERSION}"
    step "Pulling Docker image: ${image_tag}..."
    docker pull "$image_tag"

    # Create env file for reference (also used by systemd docker mode)
    step "Writing environment file: ${ENV_FILE}..."
    mkdir -p "$(dirname "$ENV_FILE")"
    cat > "$ENV_FILE" <<EOF
PORT=${PORT}
OCI_USERNAME=${USERNAME}
OCI_PASSWORD=${PASSWORD}
OCI_DB_PATH=${DB_PATH}
OCI_KEYS_DIR=${KEYS_DIR_OVERRIDE}
EOF
    chmod 600 "$ENV_FILE"

    # Write systemd unit that runs docker container
    step "Creating systemd service: ${SERVICE_FILE}..."
    cat > "$SERVICE_FILE" <<UNIT
[Unit]
Description=OCI Helper (Docker)
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
User=root
EnvironmentFile=-${ENV_FILE}
ExecStartPre=-/usr/bin/docker stop oci-helper
ExecStartPre=-/usr/bin/docker rm oci-helper
ExecStart=/usr/bin/docker run \\
  --name oci-helper \\
  --rm \\
  -p \${PORT}:\${PORT} \\
  -v ${APP_DIR}:${APP_DIR} \\
  -e PORT=\${PORT} \\
  -e OCI_USERNAME=\${OCI_USERNAME} \\
  -e OCI_PASSWORD=\${OCI_PASSWORD} \\
  -e OCI_DB_PATH=\${OCI_DB_PATH} \\
  -e OCI_KEYS_DIR=\${OCI_KEYS_DIR} \\
  ${image_tag}
ExecStop=/usr/bin/docker stop oci-helper
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
UNIT

    systemctl daemon-reload
    systemctl enable oci-helper
    systemctl start oci-helper

    info "Docker installation complete."
}

# ── Install bare-metal binary ───────────────────────────────────────────────
install_bare() {
    step "Installing bare-metal binary..."

    # Construct download URL
    local binary_name="oci-helper-${GOOS}-${GOARCH}"
    local download_url
    if [[ "$VERSION" == "latest" ]]; then
        download_url="https://github.com/${REPO}/releases/latest/download/${binary_name}"
    else
        download_url="https://github.com/${REPO}/releases/download/v${VERSION}/${binary_name}"
    fi

    # Download binary
    step "Downloading binary: ${download_url}..."
    local tmp_bin
    tmp_bin=$(mktemp)
    curl -fsSL --retry 3 --retry-delay 2 -o "$tmp_bin" "$download_url"
    if [[ ! -s "$tmp_bin" ]]; then
        # Try alternate URL format (some repos use different patterns)
        download_url="https://github.com/${REPO}/releases/download/${VERSION}/${binary_name}"
        step "Retrying with URL: ${download_url}..."
        curl -fsSL --retry 3 --retry-delay 2 -o "$tmp_bin" "$download_url"
    fi

    if [[ ! -s "$tmp_bin" ]]; then
        error "Failed to download binary. Is the version/architecture correct?"
        error "Expected binary: ${binary_name}"
        rm -f "$tmp_bin"
        exit 1
    fi

    # Install binary
    step "Installing binary to ${INSTALL_DIR}/${BINARY_NAME}..."
    chmod +x "$tmp_bin"
    mv "$tmp_bin" "${INSTALL_DIR}/${BINARY_NAME}"

    # Create directories
    step "Creating directory structure..."
    mkdir -p "$(dirname "$DB_PATH")" "$KEYS_DIR_OVERRIDE"

    # Create env file
    step "Writing environment file: ${ENV_FILE}..."
    mkdir -p "$(dirname "$ENV_FILE")"
    cat > "$ENV_FILE" <<EOF
PORT=${PORT}
OCI_USERNAME=${USERNAME}
OCI_PASSWORD=${PASSWORD}
OCI_DB_PATH=${DB_PATH}
OCI_KEYS_DIR=${KEYS_DIR_OVERRIDE}
EOF
    chmod 600 "$ENV_FILE"

    # Write systemd unit for bare-metal binary
    step "Creating systemd service: ${SERVICE_FILE}..."
    cat > "$SERVICE_FILE" <<UNIT
[Unit]
Description=OCI Helper
After=network.target

[Service]
Type=simple
User=nobody
EnvironmentFile=${ENV_FILE}
ExecStart=${INSTALL_DIR}/${BINARY_NAME}
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
UNIT

    systemctl daemon-reload
    systemctl enable oci-helper
    systemctl start oci-helper

    info "Bare-metal installation complete."
}

# ── Uninstall ────────────────────────────────────────────────────────────────
uninstall() {
    warn "This will remove oci-helper completely."
    printf "    Are you sure? [y/N] "
    read -r answer
    if [[ "${answer,,}" != "y" ]] && [[ "$answer" != "yes" ]]; then
        info "Uninstall cancelled."
        exit 0
    fi

    step "Stopping and disabling service..."
    systemctl stop oci-helper 2>/dev/null || true
    systemctl disable oci-helper 2>/dev/null || true
    rm -f "$SERVICE_FILE"
    systemctl daemon-reload 2>/dev/null || true

    step "Removing binary..."
    rm -f "${INSTALL_DIR}/${BINARY_NAME}"

    step "Removing Docker container and image..."
    if command -v docker &>/dev/null; then
        docker stop oci-helper 2>/dev/null || true
        docker rm oci-helper 2>/dev/null || true
        docker rmi "${IMAGE}:${VERSION}" 2>/dev/null || true
    fi

    step "Removing env file..."
    rm -f "$ENV_FILE"

    step "Removing data directory: ${APP_DIR}..."
    warn "Data directory ${APP_DIR} NOT removed (contains your database and keys)."
    warn "Remove it manually if desired: rm -rf ${APP_DIR}"

    info "Uninstall complete."
    exit 0
}

# ── Print summary ────────────────────────────────────────────────────────────
print_summary() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    info "${BOLD}oci-helper-go installed successfully!${NC}"
    echo ""
    echo "  ${BOLD}Access URL:${NC}  http://localhost:${PORT}"
    echo "  ${BOLD}Username:${NC}    ${USERNAME}"
    echo "  ${BOLD}Password:${NC}    ${PASSWORD}"
    echo ""
    echo "  ${BOLD}Service:${NC}     systemctl status oci-helper"
    echo "  ${BOLD}Logs:${NC}       journalctl -u oci-helper -f"
    echo "  ${BOLD}Config:${NC}     ${ENV_FILE}"
    echo ""
    printf "${YELLOW}  Store your password safely! You will need it to log in.${NC}\n"
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

# ── Main ─────────────────────────────────────────────────────────────────────
main() {
    require_root

    echo ""
    printf "${BOLD}${CYAN}  oci-helper-go installer${NC}\n"
    echo ""

    if $DO_UNINSTALL; then
        uninstall
    fi

    detect_platform
    generate_password

    # Determine installation mode
    if [[ -z "$MODE" ]]; then
        if detect_docker && [[ "$GOOS" == "linux" ]]; then
            MODE="docker"
            info "Docker detected — will install via Docker."
        else
            MODE="bare"
            if [[ "$GOOS" == "darwin" ]]; then
                info "macOS detected — will install bare-metal binary."
                warn "systemd is not available on macOS. Only the binary will be installed."
                warn "Run manually: ${INSTALL_DIR}/${BINARY_NAME}"
                warn "Or set up a launchd service manually."
            else
                info "Docker not available — will install bare-metal binary."
            fi
        fi
    fi

    check_existing
    stop_existing

    case "$MODE" in
        docker)
            if [[ "$GOOS" != "linux" ]]; then
                error "Docker mode is only supported on Linux."
                exit 1
            fi
            install_docker
            ;;
        bare)
            # On macOS, skip systemd parts
            if [[ "$GOOS" == "darwin" ]]; then
                step "Installing bare-metal binary (macOS)..."
                local binary_name="oci-helper-${GOOS}-${GOARCH}"
                local download_url
                if [[ "$VERSION" == "latest" ]]; then
                    download_url="https://github.com/${REPO}/releases/latest/download/${binary_name}"
                else
                    download_url="https://github.com/${REPO}/releases/download/v${VERSION}/${binary_name}"
                fi

                step "Downloading: ${download_url}..."
                local tmp_bin
                tmp_bin=$(mktemp)
                curl -fsSL --retry 3 --retry-delay 2 -o "$tmp_bin" "$download_url"
                if [[ ! -s "$tmp_bin" ]]; then
                    download_url="https://github.com/${REPO}/releases/download/${VERSION}/${binary_name}"
                    step "Retrying: ${download_url}..."
                    curl -fsSL --retry 3 --retry-delay 2 -o "$tmp_bin" "$download_url"
                fi
                if [[ ! -s "$tmp_bin" ]]; then
                    error "Failed to download binary. Expected: ${binary_name}"
                    rm -f "$tmp_bin"
                    exit 1
                fi
                chmod +x "$tmp_bin"
                mv "$tmp_bin" "${INSTALL_DIR}/${BINARY_NAME}"
                mkdir -p "$(dirname "$DB_PATH")" "$KEYS_DIR_OVERRIDE"

                # Write env file for reference
                mkdir -p "$(dirname "$ENV_FILE")"
                cat > "$ENV_FILE" <<EOF
PORT=${PORT}
OCI_USERNAME=${USERNAME}
OCI_PASSWORD=${PASSWORD}
OCI_DB_PATH=${DB_PATH}
OCI_KEYS_DIR=${KEYS_DIR_OVERRIDE}
EOF
                chmod 600 "$ENV_FILE"

                echo ""
                echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
                echo ""
                info "${BOLD}oci-helper-go installed successfully!${NC}"
                echo ""
                echo "  ${BOLD}Binary:${NC}  ${INSTALL_DIR}/${BINARY_NAME}"
                echo "  ${BOLD}Config:${NC}  ${ENV_FILE}"
                echo ""
                echo "  ${BOLD}Run it:${NC}"
                echo "    source ${ENV_FILE} && ${INSTALL_DIR}/${BINARY_NAME}"
                echo ""
                echo "  ${BOLD}Access URL:${NC}  http://localhost:${PORT}"
                echo "  ${BOLD}Username:${NC}    ${USERNAME}"
                echo "  ${BOLD}Password:${NC}    ${PASSWORD}"
                echo ""
                echo "  To create a launchd service, see:"
                echo "  https://github.com/viogus/oci-helper-go"
                echo ""
                echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
                exit 0
            fi
            install_bare
            ;;
    esac

    print_summary
}

main "$@"
