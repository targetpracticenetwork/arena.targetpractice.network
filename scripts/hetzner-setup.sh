#!/bin/bash

set -e

echo "🚀 Setting up Hetzner server for ArenaService..."

# Update system
echo "📦 Updating system packages..."
sudo apt update && sudo apt upgrade -y

# Install essential packages
echo "📦 Installing essential packages..."
sudo apt install -y \
    curl \
    wget \
    git \
    htop \
    vim \
    ufw \
    fail2ban \
    nginx \
    certbot \
    python3-certbot-nginx

# Configure firewall
echo "🔥 Configuring firewall..."
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow ssh
sudo ufw allow 443/tcp
sudo ufw allow 11820/udp
sudo ufw allow 6443/tcp  # k3s API
sudo ufw --force enable

# Configure fail2ban
echo "🛡️ Configuring fail2ban..."
sudo systemctl enable fail2ban
sudo systemctl start fail2ban

# Install Docker
echo "🐳 Installing Docker..."
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER

# Install k3s
echo "☸️ Installing k3s..."
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--flannel-backend=none --disable-network-policy --write-kubeconfig-mode=644" sh -

# Wait for k3s to be ready
echo "⏳ Waiting for k3s to start..."
sleep 30

# Set kubeconfig
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

# Verify k3s is running
echo "🔍 Verifying k3s installation..."
kubectl get nodes

# Install additional tools
echo "📦 Installing additional tools..."
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.2/deploy/static/provider/baremetal/deploy.yaml

# Create systemd service for ArenaService
echo "🔧 Creating systemd service..."
sudo tee /etc/systemd/system/arenaservice.service > /dev/null <<EOF
[Unit]
Description=ArenaService
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/arenaservice
ExecStart=/opt/arenaservice/arenaservice
Restart=always
RestartSec=10
Environment=WIREGUARD_INTERFACE=wg0
Environment=GRPC_LISTEN_ADDR=:443
Environment=ARENA_INSECURE_DISABLE_MTLS=false

[Install]
WantedBy=multi-user.target
EOF

# Enable services
sudo systemctl daemon-reload
sudo systemctl enable arenaservice

# Create monitoring script
echo "📊 Creating monitoring script..."
sudo tee /opt/monitor-arena.sh > /dev/null <<EOF
#!/bin/bash
echo "=== ArenaService Status ==="
kubectl get pods --all-namespaces
echo ""
echo "=== WireGuard Status ==="
wg show wg0 2>/dev/null || echo "WireGuard not running"
echo ""
echo "=== System Resources ==="
free -h
echo ""
df -h
EOF

sudo chmod +x /opt/monitor-arena.sh

# Create backup script
echo "💾 Creating backup script..."
sudo tee /opt/backup-arena.sh > /dev/null <<EOF
#!/bin/bash
BACKUP_DIR="/opt/backups/\$(date +%Y%m%d_%H%M%S)"
mkdir -p \$BACKUP_DIR

# Backup k3s data
sudo tar -czf \$BACKUP_DIR/k3s-data.tar.gz /var/lib/rancher/k3s

# Backup WireGuard config
sudo cp -r /etc/wireguard \$BACKUP_DIR/

# Backup ArenaService config
sudo cp -r /opt/arenaservice \$BACKUP_DIR/

echo "Backup created: \$BACKUP_DIR"
EOF

sudo chmod +x /opt/backup-arena.sh

# Set up log rotation
echo "📝 Setting up log rotation..."
sudo tee /etc/logrotate.d/arenaservice > /dev/null <<EOF
/var/log/arenaservice.log {
    daily
    missingok
    rotate 7
    compress
    delaycompress
    notifempty
    create 644 root root
}
EOF

echo ""
echo "🎉 Hetzner server setup complete!"
echo ""
echo "📋 Next steps:"
echo "1. Clone your ArenaService repository"
echo "2. Run: make k3s-setup"
echo "3. Configure SSL certificates"
echo "4. Set up monitoring"
echo ""
echo "🔗 Useful commands:"
echo "  /opt/monitor-arena.sh    - Check system status"
echo "  /opt/backup-arena.sh     - Create backup"
echo "  sudo systemctl status k3s"
echo "  sudo systemctl status arenaservice"
echo ""
echo "🛡️ Security:"
echo "  - Firewall configured (UFW)"
echo "  - Fail2ban enabled"
echo "  - SSH access only"
echo ""
echo "📊 Monitoring:"
echo "  - System resources: htop"
echo "  - k3s status: kubectl get nodes"
echo "  - ArenaService logs: journalctl -u arenaservice -f" 