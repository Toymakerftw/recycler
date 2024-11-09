#!/bin/bash

# Check for root privileges
if [[ $EUID -ne 0 ]]; then
    echo "This script must be run as root."
    exit 1
fi

# Define variables
INSTALL_DIR="/opt/cbin"
CBIN_PATH="/usr/local/bin/cbin"
HEALTHCHECKER_PATH="/usr/local/bin/health"
CONFIG_DIR="/etc/cbin"
LOG_DIR="/var/log/cbin"
CBINSYSTEMD_FILE="/etc/systemd/system/cbin.service"
HEALTHCHECKERSYSTEMD_FILE="/etc/systemd/system/health.service"
ENV_FILE="$CONFIG_DIR/env"
MOUNT_POINT="/mnt/recyclebin"

# Remove alias from /etc/bash.bashrc
echo "Removing alias from /etc/bash.bashrc..."
if grep -q "alias rm='$CBIN_PATH'" /etc/bash.bashrc; then
    sed -i "/alias rm='$CBIN_PATH'/d" /etc/bash.bashrc
    echo "Alias for rm command removed from /etc/bash.bashrc."
else
    echo "Alias for rm command not found in /etc/bash.bashrc."
fi

# Reload shell configuration
source /etc/bash.bashrc

# Function to stop and remove systemd services
remove_systemd_service() {
    local service_file=$1
    if [[ -f "$service_file" ]]; then
        echo "Stopping and disabling service: $service_file"
        systemctl stop $(basename "$service_file" .service)
        systemctl disable $(basename "$service_file" .service)
        rm -f "$service_file"
        systemctl daemon-reload
    else
        echo "Service file $service_file not found, skipping."
    fi
}

# Remove the binaries and symbolic links
echo "Removing binaries and symbolic links..."
rm -f "$CBIN_PATH" "$HEALTHCHECKER_PATH"
rm -rf "$INSTALL_DIR"

# Remove configuration files
echo "Removing configuration files..."
rm -f "$CONFIG_DIR/config.conf"
rm -f "$ENV_FILE"

# Remove logs
echo "Removing log directory..."
rm -rf "$LOG_DIR"

# Remove systemd services
remove_systemd_service "$CBINSYSTEMD_FILE"
remove_systemd_service "$HEALTHCHECKERSYSTEMD_FILE"

# Remove the mount entry from /etc/fstab
echo "Removing NFS mount entry from /etc/fstab..."
if grep -q "$MOUNT_POINT" /etc/fstab; then
    cp /etc/fstab /etc/fstab.bak
    grep -v "$MOUNT_POINT" /etc/fstab > /etc/fstab.tmp && mv /etc/fstab.tmp /etc/fstab
    echo "NFS mount entry removed from /etc/fstab."
else
    echo "No NFS mount entry found in /etc/fstab."
fi

# Provide a final message about the NFS mount
echo "The NFS mount point $MOUNT_POINT has not been removed to preserve your data."
echo "Please ensure that the mount is still configured as required in your /etc/fstab and remains accessible."
echo "Uninstallation complete. All relevant files and services have been removed."

# Optional: Ask the user if they want to unmount the NFS mount (they can choose to keep it)
read -p "Do you want to unmount the NFS share from $MOUNT_POINT? (y/n): " choice
if [[ "$choice" == "y" || "$choice" == "Y" ]]; then
    if mountpoint -q "$MOUNT_POINT"; then
        umount "$MOUNT_POINT"
        echo "NFS mount at $MOUNT_POINT has been unmounted."
    else
        echo "NFS mount at $MOUNT_POINT is not mounted."
    fi
else
    echo "NFS mount at $MOUNT_POINT has not been unmounted. Data is preserved."
fi
