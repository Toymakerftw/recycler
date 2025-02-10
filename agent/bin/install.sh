#!/bin/bash
set -e

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
MOUNT_POINT="/mnt/recyclebin"
CBINSYSTEMD_FILE="/etc/systemd/system/cbin.service"
HEALTHCHECKERSYSTEMD_FILE="/etc/systemd/system/health.service"
CURRENT_DIR="$(pwd)"
ENV_FILE_SRC="$CURRENT_DIR/.env"
ENV_FILE="$CONFIG_DIR/env"
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

# Function to check command success
check_success() {
    if [ $? -ne 0 ]; then
        error_exit "$1"
    fi
}

# Function to check if NFS client is installed
check_nfs_client() {
    log "Checking NFS client installation..."
    if ! command -v mount.nfs &> /dev/null; then
        log "NFS client not found. Installing..."
        if [[ -f /etc/debian_version ]]; then
            apt-get update
            apt-get install -y nfs-common
        elif [[ -f /etc/redhat-release ]]; then
            yum install -y nfs-utils
        else
            error_exit "Unsupported distribution. Please install NFS client manually."
        fi
        check_success "Failed to install NFS client"
    fi
}

# Prompt user for master server IP (only if not already set)
if [ -z "$SERVER_IP" ] && [ ! -f "$ENV_FILE" ]; then
    read -p "Enter the master server IP: " SERVER_IP
    if [ -z "$SERVER_IP" ]; then
        error_exit "Master server IP cannot be empty."
    fi

    # Ensure the CONFIG_DIR exists before writing to it
    mkdir -p "$CONFIG_DIR"

    echo "master_ip=$SERVER_IP" | tee "$ENV_FILE_SRC" "$ENV_FILE" > /dev/null
    check_success "Failed to create environment files"
elif [ -f "$ENV_FILE" ]; then
    # Load existing environment file
    source "$ENV_FILE"
    SERVER_IP=$(grep master_ip "$ENV_FILE" | cut -d '=' -f 2)
    log "Using existing master server IP: $SERVER_IP"
fi

# Create required directories (idempotent)
log "Creating directories..."
mkdir -p "$INSTALL_DIR" "$CONFIG_DIR" "$LOG_DIR" "$MOUNT_POINT"
check_success "Failed to create required directories"

# Set directory permissions (idempotent)
chown root:root "$INSTALL_DIR" "$CONFIG_DIR" "$LOG_DIR" "$MOUNT_POINT"
chmod 755 "$INSTALL_DIR" "$CONFIG_DIR"
chmod 777 "$MOUNT_POINT"
chmod 700 "$LOG_DIR"

# Set up NFS mount (idempotent)
log "Setting up NFS mount..."
if ! grep -q "$MOUNT_POINT" /etc/fstab; then
    echo "$SERVER_IP:$MOUNT_POINT $MOUNT_POINT nfs rw,sync,hard,intr 0 0" >> /etc/fstab
    check_success "Failed to update fstab"
fi

# Mount the NFS share if not already mounted
if ! mountpoint -q "$MOUNT_POINT"; then
    mount -t nfs "$SERVER_IP:$MOUNT_POINT" "$MOUNT_POINT"
    check_success "Failed to mount NFS share"
fi

# Download binaries (idempotent)
log "Downloading binaries..."
if [ ! -f "$INSTALL_DIR/cbin" ]; then
    curl -L "https://github.com/Toymakerftw/recycler/raw/refs/heads/wip/agent/bin/cbin" -o "$INSTALL_DIR/cbin"
    chmod +x "$INSTALL_DIR/cbin"
    check_success "Failed to download and set executable permissions for cbin"
fi

if [ ! -f "$INSTALL_DIR/health" ]; then
    curl -L "https://github.com/Toymakerftw/recycler/raw/refs/heads/wip/agent/bin/health" -o "$INSTALL_DIR/health"
    chmod +x "$INSTALL_DIR/health"
    check_success "Failed to download and set executable permissions for health"
fi

# Create symbolic links (idempotent)
ln -sf "$INSTALL_DIR/cbin" "$CBIN_PATH"
ln -sf "$INSTALL_DIR/health" "$HEALTHCHECKER_PATH"

# Create configuration file (idempotent)
log "Creating configuration..."
if [ ! -f "$CONFIG_DIR/config.conf" ]; then
    cat <<EOL > "$CONFIG_DIR/config.conf"
{
    "recycleBinDir": "$MOUNT_POINT",
    "numWorkers": 4,
    "checkIntervalSec": 60,
    "maxRetries": 5,
    "retryDelaySec": 60
}
EOL
    chmod 644 "$CONFIG_DIR/config.conf"
    check_success "Failed to create configuration file"
fi

# Create sudo wrapper (idempotent)
log "Creating sudo wrapper..."
if [ ! -f "$SUDO_WRAPPER" ]; then
    cat <<EOL > "$SUDO_WRAPPER"
#!/bin/bash
if [ "\$1" = "-rf" ] || [ "\$1" = "-fr" ]; then
    sudo /usr/local/bin/cbin -rf "\${@:2}" 2>/dev/null
else
    sudo /usr/local/bin/cbin "\$@" 2>/dev/null
fi
EOL
    chmod 755 "$SUDO_WRAPPER"
    check_success "Failed to create sudo wrapper"
fi

# Set up alias (idempotent)
log "Setting up alias..."
if [ ! -f "$ALIAS_FILE" ]; then
    cat <<EOL > "$ALIAS_FILE"
# CBIN rm replacement
export PATH="/usr/local/bin:\$PATH"
alias rm='/usr/local/bin/rm-wrapper'
# Apply alias immediately for current session
if [ -n "\$BASH_VERSION" ]; then
    source "$ALIAS_FILE"
fi
EOL
    chmod 644 "$ALIAS_FILE"
    check_success "Failed to create alias file"
fi

# Update shell configuration (idempotent)
for shell_rc in /etc/bash.bashrc /etc/zsh/zshrc; do
    if [ -f "$shell_rc" ]; then
        if ! grep -q "source $ALIAS_FILE" "$shell_rc"; then
            echo "source $ALIAS_FILE" >> "$shell_rc"
        fi
    fi
done

# Set SUID bit (idempotent)
chmod u+s "$CBIN_PATH"

# Create systemd services (idempotent)
log "Creating systemd services..."
if [ ! -f "$CBINSYSTEMD_FILE" ]; then
    cat <<EOL > "$CBINSYSTEMD_FILE"
[Unit]
Description=Recycler CLI Service
After=network.target nfs-client.target
Requires=nfs-client.target
[Service]
ExecStart=$CBIN_PATH
User=root
Restart=on-failure
StandardOutput=append:$LOG_DIR/cbin.log
StandardError=append:$LOG_DIR/cbin.log
[Install]
WantedBy=multi-user.target
EOL
fi

if [ ! -f "$HEALTHCHECKERSYSTEMD_FILE" ]; then
    cat <<EOL > "$HEALTHCHECKERSYSTEMD_FILE"
[Unit]
Description=CBIN Health Checker Service
After=network.target nfs-client.target
Wants=network-online.target
Requires=nfs-client.target
[Service]
Type=simple
ExecStart=$HEALTHCHECKER_PATH
Restart=always
RestartSec=10
User=root
Group=root
StandardOutput=append:$LOG_DIR/health-checker.log
StandardError=append:$LOG_DIR/health-checker.log
[Install]
WantedBy=multi-user.target
EOL
fi

# Create log files (idempotent)
touch "$LOG_DIR/cbin.log" "$LOG_DIR/health-checker.log"
chmod 600 "$LOG_DIR/cbin.log" "$LOG_DIR/health-checker.log"

# Start services (idempotent)
log "Starting services..."
systemctl daemon-reload
systemctl enable cbin
systemctl start cbin
systemctl enable health
systemctl start health

# Source alias for current session
source "$ALIAS_FILE"

log "Installation completed successfully!"
echo "Installation completed successfully!"