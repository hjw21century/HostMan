#!/bin/bash
set -e

# HostMan Agent Installer
# Usage: ./install-agent.sh -s SERVER_URL -k API_KEY [-i INTERVAL]

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

INSTALL_DIR="/opt/hostman"
CONFIG_DIR="/etc/hostman"
INTERVAL=60

usage() {
    echo "HostMan Agent Installer"
    echo ""
    echo "Usage: $0 -s SERVER_URL -k API_KEY [-i INTERVAL_SEC]"
    echo ""
    echo "Options:"
    echo "  -s  Server URL (e.g. http://10.0.0.1:8080)"
    echo "  -k  API Key (generate from server web UI)"
    echo "  -i  Report interval in seconds (default: 60)"
    echo ""
    echo "Example:"
    echo "  $0 -s http://10.0.0.1:8080 -k abc123def456"
    exit 1
}

while getopts "s:k:i:h" opt; do
    case $opt in
        s) SERVER_URL="$OPTARG" ;;
        k) API_KEY="$OPTARG" ;;
        i) INTERVAL="$OPTARG" ;;
        h) usage ;;
        *) usage ;;
    esac
done

if [ -z "$SERVER_URL" ] || [ -z "$API_KEY" ]; then
    usage
fi

echo -e "${GREEN}🖥️  HostMan Agent Installer${NC}"
echo ""

# Check root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: Please run as root${NC}"
    exit 1
fi

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64)  GOARCH="amd64" ;;
    aarch64) GOARCH="arm64" ;;
    armv7l)  GOARCH="arm" ;;
    *)
        echo -e "${RED}Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac
echo -e "  Architecture: ${YELLOW}${ARCH}${NC} (${GOARCH})"

# Create directories
echo -e "  Creating directories..."
mkdir -p "$INSTALL_DIR"
mkdir -p "$CONFIG_DIR"

# Copy binary (assumes it's in the same directory as this script)
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BINARY="${SCRIPT_DIR}/hostman-agent"

if [ ! -f "$BINARY" ]; then
    echo -e "${RED}Error: hostman-agent binary not found at ${BINARY}${NC}"
    echo -e "Build it first: go build -o bin/hostman-agent ./cmd/agent"
    exit 1
fi

echo -e "  Installing binary..."
cp "$BINARY" "${INSTALL_DIR}/hostman-agent"
chmod +x "${INSTALL_DIR}/hostman-agent"

# Write config
echo -e "  Writing config to ${CONFIG_DIR}/agent.json..."
cat > "${CONFIG_DIR}/agent.json" << EOF
{
  "server": "${SERVER_URL}",
  "api_key": "${API_KEY}",
  "interval_sec": ${INTERVAL}
}
EOF
chmod 600 "${CONFIG_DIR}/agent.json"

# Install systemd service
echo -e "  Installing systemd service..."
cat > /etc/systemd/system/hostman-agent.service << 'EOF'
[Unit]
Description=HostMan Agent - System Resource Reporter
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=/opt/hostman/hostman-agent -config /etc/hostman/agent.json
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

# Enable and start
echo -e "  Starting service..."
systemctl daemon-reload
systemctl enable hostman-agent
systemctl restart hostman-agent

# Verify
sleep 2
if systemctl is-active --quiet hostman-agent; then
    echo ""
    echo -e "${GREEN}✅ HostMan Agent installed and running!${NC}"
    echo ""
    echo "  Config:  ${CONFIG_DIR}/agent.json"
    echo "  Binary:  ${INSTALL_DIR}/hostman-agent"
    echo "  Service: hostman-agent.service"
    echo ""
    echo "  Commands:"
    echo "    systemctl status hostman-agent   # Check status"
    echo "    journalctl -u hostman-agent -f   # View logs"
    echo "    systemctl restart hostman-agent   # Restart"
    echo "    systemctl stop hostman-agent      # Stop"
else
    echo ""
    echo -e "${YELLOW}⚠️  Service installed but may not be running. Check:${NC}"
    echo "    systemctl status hostman-agent"
    echo "    journalctl -u hostman-agent -n 20"
fi
