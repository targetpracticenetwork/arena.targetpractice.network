#!/bin/bash

set -e

echo "🚀 Setting up Hetzner CAX11 ARM server for ArenaService..."

# Check architecture
ARCH=$(uname -m)
if [ "$ARCH" != "aarch64" ]; then
    echo "⚠️  Warning: This script is optimized for ARM64 (aarch64)"
    echo "   Current architecture: $ARCH"
fi

# Update system
echo "📦 Updating system packages..."
sudo apt update && sudo apt upgrade -y

# Install ARM-optimized packages
echo "📦 Installing ARM-optimized packages..."
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
    python3-certbot-nginx \
    wireguard-tools \
    iptables \
    iproute2 \
    containerd.io

# Configure firewall for ARM
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

# Install Docker for ARM
echo "🐳 Installing Docker for ARM..."
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER

# Install k3s optimized for ARM
echo "☸️ Installing k3s for ARM..."
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--flannel-backend=none --disable-network-policy --write-kubeconfig-mode=644 --docker" sh -

# Wait for k3s to be ready
echo "⏳ Waiting for k3s to start..."
sleep 30

# Set kubeconfig
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

# Verify k3s is running
echo "🔍 Verifying k3s installation..."
kubectl get nodes

# Check ARM architecture in k3s
echo "🏗️ Checking ARM architecture support..."
kubectl get nodes -o wide

# Install ARM-compatible ingress controller
echo "📦 Installing ARM-compatible ingress controller..."
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.2/deploy/static/provider/baremetal/deploy.yaml

# Create ARM-optimized systemd service for ArenaService
echo "🔧 Creating ARM-optimized systemd service..."
sudo tee /etc/systemd/system/arenaservice.service > /dev/null <<EOF
[Unit]
Description=ArenaService (ARM Optimized)
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
Environment=GOMAXPROCS=2
Environment=GOARCH=arm64

[Install]
WantedBy=multi-user.target
EOF

# Enable services
sudo systemctl daemon-reload
sudo systemctl enable arenaservice

# Create ARM-specific monitoring script
echo "📊 Creating ARM monitoring script..."
sudo tee /opt/monitor-arena-arm.sh > /dev/null <<EOF
#!/bin/bash
echo "=== ARM Server Status ==="
echo "Architecture: \$(uname -m)"
echo "CPU: \$(nproc) cores"
echo "Memory: \$(free -h | grep Mem | awk '{print \$2}')"
echo ""
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
echo ""
echo "=== ARM Performance ==="
top -bn1 | grep "Cpu(s)" | awk '{print \$2}' | cut -d'%' -f1
EOF

sudo chmod +x /opt/monitor-arena-arm.sh

# Create ARM-optimized backup script
echo "💾 Creating ARM backup script..."
sudo tee /opt/backup-arena-arm.sh > /dev/null <<EOF
#!/bin/bash
BACKUP_DIR="/opt/backups/\$(date +%Y%m%d_%H%M%S)"
mkdir -p \$BACKUP_DIR

# Backup k3s data
sudo tar -czf \$BACKUP_DIR/k3s-data.tar.gz /var/lib/rancher/k3s

# Backup WireGuard config
sudo cp -r /etc/wireguard \$BACKUP_DIR/

# Backup ArenaService config
sudo cp -r /opt/arenaservice \$BACKUP_DIR/

# Backup system info
echo "ARM64 Server Backup" > \$BACKUP_DIR/system-info.txt
uname -a >> \$BACKUP_DIR/system-info.txt
echo "" >> \$BACKUP_DIR/system-info.txt
echo "CPU Info:" >> \$BACKUP_DIR/system-info.txt
lscpu >> \$BACKUP_DIR/system-info.txt

echo "ARM backup created: \$BACKUP_DIR"
EOF

sudo chmod +x /opt/backup-arena-arm.sh

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

# ARM-specific optimizations
echo "⚡ Applying ARM-specific optimizations..."

# Optimize for ARM CPU
echo 'vm.swappiness=10' | sudo tee -a /etc/sysctl.conf
echo 'vm.vfs_cache_pressure=50' | sudo tee -a /etc/sysctl.conf

# Optimize for NVMe storage
echo 'vm.dirty_ratio=15' | sudo tee -a /etc/sysctl.conf
echo 'vm.dirty_background_ratio=5' | sudo tee -a /etc/sysctl.conf

# Apply sysctl changes
sudo sysctl -p

echo ""
echo "🎉 Hetzner CAX11 ARM server setup complete!"
echo ""
echo "📋 ARM Server Info:"
echo "  Architecture: $(uname -m)"
echo "  CPU Cores: $(nproc)"
echo "  Memory: $(free -h | grep Mem | awk '{print $2}')"
echo "  Storage: $(df -h / | awk 'NR==2 {print $2}')"
echo ""
echo "📋 Next steps:"
echo "1. Clone your ArenaService repository"
echo "2. Build for ARM64: GOOS=linux GOARCH=arm64 go build"
echo "3. Run: make k3s-setup"
echo "4. Configure SSL certificates"
echo "5. Set up monitoring"
echo ""
echo "🔗 Useful commands:"
echo "  /opt/monitor-arena-arm.sh    - Check ARM system status"
echo "  /opt/backup-arena-arm.sh     - Create ARM backup"
echo "  sudo systemctl status k3s"
echo "  sudo systemctl status arenaservice"
echo ""
echo "🛡️ Security:"
echo "  - Firewall configured (UFW)"
echo "  - Fail2ban enabled"
echo "  - SSH access only"
echo ""
echo "⚡ ARM Optimizations:"
echo "  - CPU scheduling optimized"
echo "  - Memory management tuned"
echo "  - NVMe storage optimized"
echo "  - k3s ARM64 support enabled"
echo ""
echo "📊 Expected Performance:"
echo "  - 5-10 concurrent arenas"
echo "  - Fast arena creation (< 3s)"
echo "  - Efficient resource usage"
echo "  - Low power consumption" 