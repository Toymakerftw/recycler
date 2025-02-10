#!/bin/bash
set -e

# Check for root privileges
if [[ $EUID -ne 0 ]]; then
    echo "This script must be run as root."
    exit 1
fi

# Define variables (matching the installation script)
INSTALL_DIR="/opt/cbin"
CBIN_PATH="/usr/local/bin/cbin"
HEALTHCHECKER_PATH="/usr/local/bin/health"
CONFIG_DIR="/etc/cbin"
LOG_DIR="/var/log/cbin"
MOUNT_POINT="/mnt/recyclebin"
CBINSYSTEMD_FILE="/etc/systemd/system/cbin.service"
HEALTHCHECKERSYSTEMD_FILE="/etc/systemd/system/health.service"
ENV_FILE="/etc/cbin/env"
ALIAS_FILE="/etc/profile.d/cbin.sh"
SUDO_WRAPPER="/usr/local/bin/rm-wrapper"

# Function to log messages
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

# Function to handle errors
error_exit() {
    log "ERROR: $1" >&2
    exit 1
}

# Stop and disable services
log "Stopping and disabling services..."
systemctl stop cbin || true
systemctl disable cbin || true
systemctl stop health || true
systemctl disable health || true

# Remove systemd service files
log "Removing systemd service files..."
rm -f "$CBINSYSTEMD_FILE" "$HEALTHCHECKERSYSTEMD_FILE"
systemctl daemon-reload

# Unmount NFS share if mounted
log "Unmounting NFS share..."
if mountpoint -q "$MOUNT_POINT"; then
    umount "$MOUNT_POINT" || error_exit "Failed to unmount NFS share"
fi

# Remove NFS entry from /etc/fstab
log "Removing NFS entry from /etc/fstab..."
sed -i "\|$MOUNT_POINT|d" /etc/fstab

# Remove installed binaries and symbolic links
log "Removing binaries and symbolic links..."
rm -f "$INSTALL_DIR/cbin" "$INSTALL_DIR/health" "$CBIN_PATH" "$HEALTHCHECKER_PATH"

# Remove configuration files and directories
log "Removing configuration files and directories..."
rm -rf "$CONFIG_DIR" "$LOG_DIR" "$MOUNT_POINT"

# Remove sudo wrapper
log "Removing sudo wrapper..."
rm -f "$SUDO_WRAPPER"

# Remove alias file
log "Removing alias file..."
rm -f "$ALIAS_FILE"

# Remove alias sourcing from shell configuration files
log "Removing alias sourcing from shell configuration files..."
for shell_rc in /etc/bash.bashrc /etc/zsh/zshrc; do
    if [ -f "$shell_rc" ]; then
        sed -i "/source $ALIAS_FILE/d" "$shell_rc"
    fi
done

# Remove environment file
log "Removing environment file..."
rm -f "$ENV_FILE"

# Log completion
log "Uninstallation completed successfully!"
echo "Uninstallation completed successfully!"