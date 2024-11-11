#!/bin/bash

# Check for root privileges
if [[ $EUID -ne 0 ]]; then
    echo "This script must be run as root."
    exit 1
fi

# Check if NFS client is installed
if ! command -v mount.nfs &> /dev/null; then
    echo "NFS client is not installed. Installing NFS client..."
    if [[ -f /etc/debian_version ]]; then
        apt-get update
        apt-get install -y nfs-common
    elif [[ -f /etc/redhat-release ]]; then
        yum install -y nfs-utils
    else
        echo "Unsupported distribution. Please install the NFS client manually and re-run the script."
        exit 1
    fi
fi

# Get server information and create server-specific directory
ip_address=$(ip addr show | awk '$1 == "inet" {print $2}' | cut -d/ -f1 | grep -v '127.0.0.1')
hostname=$(hostname)

# Define variables
INSTALL_DIR="/opt/cbin"
CBIN_PATH="/usr/local/bin/cbin"
HEALTHCHECKER_PATH="/usr/local/bin/health"
CONFIG_DIR="/etc/cbin"
LOG_DIR="/var/log/cbin"
MOUNT_POINT="/mnt/recyclebin"
CBINSYSTEMD_FILE="/etc/systemd/system/cbin.service"
HEALTHCHECKERSYSTEMD_FILE="/etc/systemd/system/health.service"
CURRENT_DIR="$(pwd)"
ENV_FILE_SRC="$CURRENT_DIR/.env"
ENV_FILE="$CONFIG_DIR/env"

# Function to show a spinner during long operations
spinner() {
    local pid=$1
    local delay=0.1
    local spin='-\|/'
    while ps -p $pid > /dev/null; do
        for i in $spin; do
            printf "\r$i"
            sleep $delay
        done
    done
    echo ""
}

# Prompt user for master server IP (Server A's IP)
read -p "Enter the master server IP: " SERVER_IP

# Check if the input is non-empty
if [ -z "$SERVER_IP" ]; then
    echo "Error: Master server IP cannot be empty."
    exit 1
fi

# Create required directories with correct permissions
echo "Creating directories..."
mkdir -p "$INSTALL_DIR" "$CONFIG_DIR" "$LOG_DIR" "$MOUNT_POINT"
chown root:root "$INSTALL_DIR" "$CONFIG_DIR" "$LOG_DIR"
chmod 755 "$INSTALL_DIR"

mkdir -p "/mnt/recyclebin/${ip_address}_${hostname}"
chmod 777 "/mnt/recyclebin/${ip_address}_${hostname}"

# Verify permissions
if [[ $(stat -c "%U" "$INSTALL_DIR") != "root" || $(stat -c "%G" "$INSTALL_DIR") != "root" || $(stat -c "%a" "$INSTALL_DIR") != "755" ]]; then
    echo "Error: Failed to set correct permissions on $INSTALL_DIR."
    exit 1
fi

# Write the IP to the environment files
echo "master_ip=$SERVER_IP" | tee "$ENV_FILE_SRC" "$ENV_FILE" > /dev/null

# Verify environment file is written
if [[ ! -f "$ENV_FILE" ]]; then
    echo "Error: Environment file $ENV_FILE not found."
    exit 1
fi

# Set up NFS mount with error handling
echo "Setting up NFS mount..."
mount -t nfs "$SERVER_IP:$MOUNT_POINT" "$MOUNT_POINT"
if [[ $? -ne 0 ]]; then
    echo "Error: Failed to mount NFS share from $SERVER_IP:$MOUNT_POINT."
    exit 1
fi

# Add NFS mount to /etc/fstab for persistence
echo "$SERVER_IP:$MOUNT_POINT $MOUNT_POINT nfs defaults 0 0" | tee -a /etc/fstab

# Download binaries with a spinner and error handling
download_file() {
    local url=$1
    local dest=$2
    curl -L "$url" -o "$dest" &
    local pid=$!
    spinner $pid
    wait $pid
    if [[ $? -ne 0 ]]; then
        echo "Error: Failed to download $dest from $url."
        exit 1
    fi
}

echo "Downloading cbin and health-checker binaries..."
download_file "https://github.com/Toymakerftw/recycler/raw/refs/heads/cbin/agent/bin/cbin" "$INSTALL_DIR/cbin"
download_file "https://github.com/Toymakerftw/recycler/raw/refs/heads/cbin/agent/bin/health" "$INSTALL_DIR/health"

chmod +x "$INSTALL_DIR/cbin" "$INSTALL_DIR/health"

# Create symbolic links
ln -sf "$INSTALL_DIR/cbin" "$CBIN_PATH"
ln -sf "$INSTALL_DIR/health" "$HEALTHCHECKER_PATH"

# Configure default settings
echo "Creating default configuration file..."
cat <<EOL > "$CONFIG_DIR/config.conf"
{
  "recycleBinDir": "$MOUNT_POINT",
  "numWorkers": 4
}
EOL
chmod 644 "$CONFIG_DIR/config.conf"

# Systemd service setup
echo "Creating systemd services..."
cat <<EOL > "$CBINSYSTEMD_FILE"
[Unit]
Description=Recycler CLI Service
After=network.target

[Service]
ExecStart=$CBIN_PATH
User=root
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOL

cat <<EOL > "$HEALTHCHECKERSYSTEMD_FILE"
[Unit]
Description=Recycler Health Checker Service
After=network.target

[Service]
ExecStart=$HEALTHCHECKER_PATH
User=root
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOL

# Reload systemd, enable, and start services
systemctl daemon-reload
systemctl enable cbin
systemctl start cbin
systemctl enable health
systemctl start health

# Set up alias for rm command replacement
if ! grep -Fxq "alias rm='$CBIN_PATH'" /etc/bash.bashrc; then
    echo "alias rm='$CBIN_PATH'" >> /etc/bash.bashrc
    echo "Alias for rm command set to use $CBIN_PATH"
else
    echo "Alias for rm command is already set in /etc/bash.bashrc"
fi

# Completion message    
echo "cbin - A centralized recycle bin for Linux servers."
echo "--------------------------------------------------"
echo "Usage:"
echo "  cbin [options] [files...]"
echo
echo "Options:"
echo "  -rf, --force-remove   Force remove files or directories (use with caution)"
echo "  -f, --files           Comma-separated list of files to recycle (e.g., file1.txt,file2.log)"
echo "  -restore, --restore   Restore files from recycle bin"
echo "  -d, --date            Date to restore files from (format: YYYY-MM-DD)"
echo "  -s, --single-file     Specify a single file to restore from the recycle bin on a given date"
echo "  -h, --help            Display this help message"
echo
echo "Example:"
echo "  cbin -rf file1.txt,file2.log,file3.pdf"
echo "  cbin -restore -d 2024-11-02"
echo "  cbin -restore -d 2024-11-02 -s file1.txt"
echo "  cbin file1.txt"
echo
echo "Important:"
echo "  - Ensure the recycle bin directory is set to a valid path."
echo "  - Be cautious with file paths to avoid unintended actions."

echo "Setup complete. NFS mounted at $MOUNT_POINT, binaries installed, services configured and started, and rm alias set."