#!/bin/sh
set -e

INSTALL_DIR="/data/drivers/shm-et340"
SERVICE_DIR="${INSTALL_DIR}/service"
SERVICE_LINK="/service/shm-et340"
RC_LOCAL="/data/rc.local"
REPO="mitchese/shm-et340"

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    armv7l|armv6l|arm) ARCH="arm" ;;
    aarch64)           ARCH="arm64" ;;
    x86_64)            ARCH="amd64" ;;
    *)
        echo "ERROR: Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

# Fetch latest release tag from GitHub API
echo "Fetching latest release..."
LATEST=$(wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')

if [ -z "$LATEST" ]; then
    echo "ERROR: Could not determine latest release. Check your internet connection."
    exit 1
fi
echo "Latest release: $LATEST"

BINARY_URL="https://github.com/${REPO}/releases/download/${LATEST}/shm-et340"
SERVICE_URL="https://raw.githubusercontent.com/${REPO}/master/service/run"

# Stop existing service if running
if [ -L "$SERVICE_LINK" ]; then
    echo "Stopping existing service..."
    sv stop shm-et340 2>/dev/null || true
    sleep 2
fi

# Create directories
mkdir -p "$SERVICE_DIR"

# Download binary
echo "Downloading shm-et340 ${LATEST} (${ARCH})..."
wget -qO "${INSTALL_DIR}/shm-et340.new" "$BINARY_URL"
chmod +x "${INSTALL_DIR}/shm-et340.new"

# Swap binary (atomic-ish — old binary kept as .old for rollback)
if [ -f "${INSTALL_DIR}/shm-et340" ]; then
    mv "${INSTALL_DIR}/shm-et340" "${INSTALL_DIR}/shm-et340.old"
fi
mv "${INSTALL_DIR}/shm-et340.new" "${INSTALL_DIR}/shm-et340"

# Download service script only if it doesn't already exist (preserve user edits)
if [ ! -f "${SERVICE_DIR}/run" ]; then
    echo "Downloading service script..."
    wget -qO "${SERVICE_DIR}/run" "$SERVICE_URL"
    chmod +x "${SERVICE_DIR}/run"
else
    echo "Service script already exists, keeping existing version."
    echo "To update it: wget -qO ${SERVICE_DIR}/run ${SERVICE_URL}"
fi

# Create service symlink if not already present
if [ ! -L "$SERVICE_LINK" ]; then
    echo "Creating service symlink..."
    ln -s "$SERVICE_DIR" "$SERVICE_LINK"
fi

# Add to rc.local for firmware update survival (idempotent)
if [ ! -f "$RC_LOCAL" ] || ! grep -q "shm-et340" "$RC_LOCAL"; then
    echo "Adding to ${RC_LOCAL} for firmware update survival..."
    echo "" >> "$RC_LOCAL"
    echo "ln -s ${SERVICE_DIR} ${SERVICE_LINK}" >> "$RC_LOCAL"
    chmod +x "$RC_LOCAL"
fi

# Start the service
echo "Starting service..."
sv start shm-et340 2>/dev/null || true

echo ""
echo "Installation complete."
echo "  Binary:  ${INSTALL_DIR}/shm-et340"
echo "  Service: ${SERVICE_DIR}/run"
echo ""
echo "The service will start automatically and survive firmware updates."
echo "To check status:  sv status shm-et340"
echo "To view logs:     cat /var/log/shm-et340/current"
echo "To run diagnostics: ${INSTALL_DIR}/shm-et340 --diagnose"
