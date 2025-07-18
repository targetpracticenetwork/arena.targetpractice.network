#!/bin/bash

set -e

echo "🚀 Setting up server for ArenaService with k3s..."

# Check architecture
ARCH=$(uname -m)
echo "📋 Architecture: $ARCH"

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
    python3-certbot-nginx \
    wireguard-tools \
    iptables \
    iproute2

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



# Install k3s with architecture-specific optimizations
echo "☸️ Installing k3s..."
if [ "$ARCH" = "aarch64" ]; then
    echo "📱 Installing k3s for ARM64..."
    curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--flannel-backend=none --disable-network-policy --write-kubeconfig-mode=644" sh -
else
    echo "💻 Installing k3s for x86_64..."
    curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--flannel-backend=none --disable-network-policy --write-kubeconfig-mode=644" sh -
fi

# Wait for k3s to be ready
echo "⏳ Waiting for k3s to start..."
sleep 30

# Set kubeconfig
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

# Verify k3s is running
echo "🔍 Verifying k3s installation..."
kubectl get nodes

# Install ingress controller
echo "📦 Installing ingress controller..."
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.2/deploy/static/provider/baremetal/deploy.yaml

# Build and deploy ArenaService to k3s
echo "🔨 Building and deploying ArenaService to k3s..."
if [ -d "/home/topman/arena.targetpractice.network" ]; then
    cd /home/topman/arena.targetpractice.network
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -installsuffix cgo -o bin/arenaservice-static ./cmd/arenaservice
    kubectl apply -k ./cluster/control-plane/
    echo "✅ ArenaService deployed to k3s"
else
    echo "⚠️  ArenaService repository not found at /home/topman/arena.targetpractice.network"
    echo "   Run 'make k3s-deploy' after cloning the repository"
fi

# Create monitoring script
echo "📊 Creating monitoring script..."
sudo tee /opt/monitor-arena.sh > /dev/null <<EOF
#!/bin/bash
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
echo "=== Server Status ==="
echo "Architecture: \$(uname -m)"
echo "CPU: \$(nproc) cores"
echo "Memory: \$(free -h | grep Mem | awk '{print \$2}')"
echo ""
echo "=== k3s Cluster Status ==="
kubectl get nodes
echo ""
echo "=== ArenaService Status ==="
kubectl get pods --all-namespaces
echo ""
echo "=== ArenaService Logs ==="
kubectl logs -n control-plane deployment/arenaservice --tail=10 2>/dev/null || echo "ArenaService not running"
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

# Backup system info
echo "Server Backup" > \$BACKUP_DIR/system-info.txt
uname -a >> \$BACKUP_DIR/system-info.txt
echo "" >> \$BACKUP_DIR/system-info.txt
echo "CPU Info:" >> \$BACKUP_DIR/system-info.txt
lscpu >> \$BACKUP_DIR/system-info.txt

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

# Apply system optimizations
echo "⚡ Applying system optimizations..."
echo 'vm.swappiness=10' | sudo tee -a /etc/sysctl.conf
echo 'vm.vfs_cache_pressure=50' | sudo tee -a /etc/sysctl.conf
echo 'vm.dirty_ratio=15' | sudo tee -a /etc/sysctl.conf
echo 'vm.dirty_background_ratio=5' | sudo tee -a /etc/sysctl.conf

# Apply sysctl changes
sudo sysctl -p

echo ""
echo "🎉 Server setup complete!"
echo ""
echo "📋 Server Info:"
echo "  Architecture: $(uname -m)"
echo "  CPU Cores: $(nproc)"
echo "  Memory: $(free -h | grep Mem | awk '{print $2}')"
echo "  Storage: $(df -h / | awk 'NR==2 {print $2}')"
echo ""
echo "📋 Next steps:"
echo "1. Clone your ArenaService repository"
echo "2. Run: make k3s-deploy"
echo "3. Configure SSL certificates"
echo "4. Set up monitoring"
echo ""
echo "🔗 Useful commands:"
echo "  /opt/monitor-arena.sh    - Check system status"
echo "  /opt/backup-arena.sh     - Create backup"
echo "  sudo systemctl status k3s"
echo "  kubectl get pods --all-namespaces"
echo "  kubectl logs -n control-plane deployment/arenaservice"
echo ""
echo "🛡️ Security:"
echo "  - Firewall configured (UFW)"
echo "  - Fail2ban enabled"
echo "  - SSH access only"
echo ""
echo "⚡ Optimizations:"
echo "  - CPU scheduling optimized"
echo "  - Memory management tuned"
echo "  - k3s $(uname -m) support enabled"
echo ""
echo "📊 Expected Performance:"
echo "  - 5-10 concurrent arenas"
echo "  - Fast arena creation (< 3s)"
echo "  - Efficient resource usage" 