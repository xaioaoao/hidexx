#!/bin/bash
# hidexx one-click deploy script
# usage: curl -sL https://raw.githubusercontent.com/xaioaoao/hidexx/master/deploy.sh | bash

set -e

echo "=== hidexx one-click deploy ==="
echo ""

# install dependencies
echo "[1/5] installing dependencies..."
sudo apt update -qq && sudo apt install -y -qq tesseract-ocr git > /dev/null 2>&1
echo "  done"

# install go
if ! command -v go &> /dev/null; then
    echo "[2/5] installing Go..."
    curl -sLO https://go.dev/dl/go1.21.13.linux-amd64.tar.gz
    sudo tar -C /usr/local -xzf go1.21.13.linux-amd64.tar.gz
    rm -f go1.21.13.linux-amd64.tar.gz
    echo "  done"
else
    echo "[2/5] Go already installed, skip"
fi
export PATH=$PATH:/usr/local/go/bin

# make sure go is in PATH permanently
grep -q '/usr/local/go/bin' /etc/profile || echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee -a /etc/profile > /dev/null

# clone and build
echo "[3/5] building hidexx..."
cd ~
rm -rf hidexx
git clone -q https://github.com/xaioaoao/hidexx.git
cd hidexx
go build -o hidexx .
sudo cp hidexx /usr/local/bin/
echo "  done"

# stop old services
echo "[4/5] setting up services..."
sudo systemctl stop hidexx-serve 2>/dev/null || true
sudo systemctl stop hidexx-ss 2>/dev/null || true
sudo fuser -k 51800/tcp 2>/dev/null || true
sudo fuser -k 51801/tcp 2>/dev/null || true
sudo fuser -k 51802/tcp 2>/dev/null || true
sudo fuser -k 51991/tcp 2>/dev/null || true

# create systemd services
sudo tee /etc/systemd/system/hidexx-serve.service > /dev/null << 'EOF'
[Unit]
Description=Hidexx subscription crawler
After=network.target

[Service]
ExecStart=/usr/local/bin/hidexx serve -n 2 -p 51991
Restart=always
RestartSec=10
WorkingDirectory=/root

[Install]
WantedBy=multi-user.target
EOF

sudo tee /etc/systemd/system/hidexx-ss.service > /dev/null << 'EOF'
[Unit]
Description=Hidexx Shadowsocks proxy
After=network.target

[Service]
ExecStart=/usr/local/bin/hidexx ss -n 2
Restart=always
RestartSec=10
WorkingDirectory=/root

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now hidexx-serve
sudo systemctl enable --now hidexx-ss
echo "  done"

# wait for startup
echo "[5/5] waiting for services to start..."
sleep 30

# get public ip
PUBLIC_IP=$(curl -s https://api.ipify.org || echo "YOUR_SERVER_IP")

echo ""
echo "=========================================="
echo "  hidexx deployed successfully!"
echo "=========================================="
echo ""
echo "  Public IP: $PUBLIC_IP"
echo ""
echo "  --- crawled subscriptions (auto-renew daily) ---"
echo "  user 1: http://$PUBLIC_IP:51991/1/sub.yaml"
echo "  user 2: http://$PUBLIC_IP:51991/2/sub.yaml"
echo ""
echo "  --- Shadowsocks proxy (permanent) ---"
echo "  Clash config:"
echo "  user 1: http://$PUBLIC_IP:51800/1/clash.yaml"
echo "  user 2: http://$PUBLIC_IP:51800/2/clash.yaml"
echo ""
echo "  SS one-click links:"
sudo journalctl -u hidexx-ss --no-pager -n 20 2>/dev/null | grep "one-click" || cat /tmp/ss.log 2>/dev/null | grep "one-click" || echo "  (check: sudo journalctl -u hidexx-ss)"
echo ""
echo "  --- management ---"
echo "  status:  sudo systemctl status hidexx-serve hidexx-ss"
echo "  logs:    sudo journalctl -fu hidexx-serve"
echo "           sudo journalctl -fu hidexx-ss"
echo "  restart: sudo systemctl restart hidexx-serve hidexx-ss"
echo ""
echo "  NOTE: open firewall ports 51800, 51801, 51802, 51991 (TCP)"
echo "=========================================="
