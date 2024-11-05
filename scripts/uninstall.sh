#!/bin/bash

# Define variables
INSTALL_DIR="/opt/recycler-cli"
BIN_PATH="/usr/local/bin/recycler-cli"
CONFIG_DIR="/etc/recycler-cli"
LOG_DIR="/var/log/recycler-cli"
RECYCLE_BIN_DIR="/mnt/recycle-bin"
SYSTEMD_FILE="/etc/systemd/system/recycler-cli.service"

echo "Uninstalling recycler-cli..."

# Stop and disable the systemd service if it exists
if [ -f "$SYSTEMD_FILE" ]; then
    echo "Stopping and disabling the recycler-cli service..."
    sudo systemctl stop recycler-cli
    sudo systemctl disable recycler-cli
    sudo rm -f "$SYSTEMD_FILE"
    sudo systemctl daemon-reload
fi

# Remove symbolic link from /usr/local/bin
if [ -L "$BIN_PATH" ]; then
    echo "Removing symbolic link..."
    sudo rm -f "$BIN_PATH"
fi

# Remove installation directory
if [ -d "$INSTALL_DIR" ]; then
    echo "Removing installation directory..."
    sudo rm -rf "$INSTALL_DIR"
fi

# Remove configuration directory
if [ -d "$CONFIG_DIR" ]; then
    echo "Removing configuration directory..."
    sudo rm -rf "$CONFIG_DIR"
fi

# Optionally remove log directory and recycle bin directory (confirm with the user)
read -p "Do you want to remove the log directory ($LOG_DIR)? [y/N]: " remove_log_dir
if [[ "$remove_log_dir" == "y" || "$remove_log_dir" == "Y" ]]; then
    echo "Removing log directory..."
    sudo rm -rf "$LOG_DIR"
fi

read -p "Do you want to remove the recycle bin directory ($RECYCLE_BIN_DIR)? [y/N]: " remove_recycle_bin
if [[ "$remove_recycle_bin" == "y" || "$remove_recycle_bin" == "Y" ]]; then
    echo "Removing recycle bin directory..."
    sudo rm -rf "$RECYCLE_BIN_DIR"
fi

echo "Recycler-cli has been uninstalled successfully."
