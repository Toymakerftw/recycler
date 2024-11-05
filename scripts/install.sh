#!/bin/bash

# Define variables
INSTALL_DIR="/opt/recycler-cli"
BIN_PATH="/usr/local/bin/recycler-cli"
CONFIG_DIR="/etc/recycler-cli"
LOG_DIR="/var/log/recycler-cli"
RECYCLE_BIN_DIR="/mnt/recycle-bin"
SYSTEMD_FILE="/etc/systemd/system/recycler-cli.service"

# Create necessary directories
echo "Creating installation directories..."
sudo mkdir -p "$INSTALL_DIR" "$CONFIG_DIR" "$LOG_DIR" "$RECYCLE_BIN_DIR"

# Download binary from GitHub and place it in the installation directory
echo "Downloading recycler-cli binary..."
sudo curl -L "https://github.com/Toymakerftw/recycler/raw/refs/heads/go/bin/recycler-cli" -o "$INSTALL_DIR/recycler-cli"
sudo chmod +x "$INSTALL_DIR/recycler-cli"

# Create symbolic link to make it globally accessible
echo "Creating symbolic link for global access..."
sudo ln -sf "$INSTALL_DIR/recycler-cli" "$BIN_PATH"

# Set up default configuration file
echo "Setting up default configuration file..."
cat <<EOL | sudo tee "$CONFIG_DIR/config.conf" > /dev/null
{
  "recycleBinDir": "$RECYCLE_BIN_DIR",
  "numWorkers": 4
}
EOL

# Adjust ownership and permissions for the log and recycle bin directories
echo "Adjusting permissions for log and recycle bin directories..."
sudo chown -R "$USER":"$USER" "$LOG_DIR" "$RECYCLE_BIN_DIR"
sudo chmod 755 "$LOG_DIR" "$RECYCLE_BIN_DIR"

# Set up systemd service for background running (optional)
echo "Creating systemd service..."
cat <<EOL | sudo tee "$SYSTEMD_FILE" > /dev/null
[Unit]
Description=Recycler CLI Service
After=network.target

[Service]
ExecStart=$BIN_PATH
User=$USER
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOL

# Reload systemd, enable and start the service
sudo systemctl daemon-reload
sudo systemctl enable recycler-cli
sudo systemctl start recycler-cli

# Display configuration and log file locations, along with usage information
echo -e "\nInstallation Complete!"
echo -e "---------------------------------------------------------------"
echo -e "Recycler CLI is now installed and globally accessible as 'recycler-cli'"
echo -e "Default Configuration File: $CONFIG_DIR/config.conf"
echo -e "Log File: $LOG_DIR/recycler-cli.log"
echo -e "Recycle Bin Directory: $RECYCLE_BIN_DIR"
echo -e "---------------------------------------------------------------"
echo -e "Usage:"
echo -e "  recycler-cli -f <file1,file2,...>     # Move specified files to recycle bin"
echo -e "  recycler-cli -h                       # Display help message"
echo -e "Example:"
echo -e "  recycler-cli -f file1.txt,file2.log"
echo -e "---------------------------------------------------------------"
echo -e "The tool will automatically recycle the specified files to the configured directory.\n"
