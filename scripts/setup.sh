#!/bin/bash

set -e

echo "🚀 Complete ArenaService setup (Server + k3s + Deployment)..."

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
    wireguard-tools \
    iptables \
    iproute2

# Configure firewall
echo "🔥 Configuring firewall..."
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow ssh
sudo ufw allow 443/tcp    # ArenaService gRPC
sudo ufw allow 11820/udp  # WireGuard
sudo ufw allow 6443/tcp   # k3s API
sudo ufw --force enable

# Configure fail2ban
echo "🛡️ Configuring fail2ban..."
sudo systemctl enable fail2ban
sudo systemctl start fail2ban

# Install k3s
echo "☸️ Installing k3s..."
if command -v k3s > /dev/null 2>&1; then
    echo "✅ k3s is already installed"
else
    echo "📦 Installing k3s..."
    # Disable traefik since ArenaService uses LoadBalancer service directly
    if [ "$ARCH" = "aarch64" ]; then
        echo "📱 Installing k3s for ARM64..."
        curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--disable=traefik --write-kubeconfig-mode=644" sh -
    else
        echo "💻 Installing k3s for x86_64..."
        curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--disable=traefik --write-kubeconfig-mode=644" sh -
    fi
    
    # Wait for k3s to be ready
    echo "⏳ Waiting for k3s to start..."
    sleep 30
fi

# Set kubeconfig
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
sudo chmod 644 $KUBECONFIG

# Verify k3s is running
echo "🔍 Verifying k3s installation..."
kubectl get nodes
kubectl get pods --all-namespaces

# Build and deploy ArenaService
echo "🔨 Building and deploying ArenaService..."
if [ -f "go.mod" ]; then
    # Build static binary
    echo "🔨 Building static binary..."
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -installsuffix cgo -o bin/arenaservice-static ./cmd/arenaservice
    
    # Deploy ArenaService cluster configuration
    echo "🏗️ Applying ArenaService cluster configuration..."
    
    # Control plane (ArenaService with integrated WireGuard)
    echo "🎮 Applying control plane..."
    kubectl apply -k ./cluster/control-plane
    
    # Arena area baseline configuration
    echo "🏟️ Applying arena area..."
    kubectl apply -k ./cluster/arena-area
    
    # Wait for all pods to be ready
    echo "⏳ Waiting for ArenaService to be ready..."
    kubectl wait --for=condition=available --timeout=300s deployment/arenaservice -n control-plane
    
    echo "✅ ArenaService deployed successfully!"
else
    echo "⚠️  ArenaService repository not found (no go.mod)"
    echo "   Please run this script from the ArenaService root directory"
fi

# Create monitoring script
echo "📊 Creating monitoring script..."
sudo tee /opt/monitor-arena.sh > /dev/null <<'EOF'
#!/bin/bash
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
echo "=== Server Status ==="
echo "Architecture: $(uname -m)"
echo "CPU: $(nproc) cores"
echo "Memory: $(free -h | grep Mem | awk '{print $2}')"
echo ""
echo "=== k3s Cluster Status ==="
kubectl get nodes
echo ""
echo "=== ArenaService Status ==="
kubectl get pods -n control-plane
kubectl get service -n control-plane
echo ""
echo "=== WireGuard Status ==="
kubectl exec -n control-plane deployment/arenaservice -- ip link show wg0 2>/dev/null || echo "WireGuard interface not ready"
echo ""
echo "=== Recent Logs ==="
kubectl logs -n control-plane deployment/arenaservice --tail=5
EOF

sudo chmod +x /opt/monitor-arena.sh

# Create backup script
echo "💾 Creating backup script..."
sudo tee /opt/backup-arena.sh > /dev/null <<'EOF'
#!/bin/bash
BACKUP_DIR="/opt/arena-backups"
DATE=$(date +%Y%m%d_%H%M%S)
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

mkdir -p $BACKUP_DIR

echo "🗄️ Backing up ArenaService configuration..."
kubectl get all -n control-plane -o yaml > $BACKUP_DIR/arenaservice_$DATE.yaml
kubectl get configmaps,secrets -n control-plane -o yaml > $BACKUP_DIR/config_$DATE.yaml

echo "✅ Backup completed: $BACKUP_DIR/"
ls -la $BACKUP_DIR/
EOF

sudo chmod +x /opt/backup-arena.sh

echo ""
echo "🎉 Complete ArenaService setup finished!"
echo ""
echo "📋 Cluster info:"
kubectl cluster-info
echo ""
echo "📋 Pod Status:"
kubectl get pods -n control-plane
echo ""
echo "📋 Service (LoadBalancer):"
kubectl get service -n control-plane
echo ""
echo "🔗 Access points:"
echo "  Kubernetes API: https://localhost:6443"
echo "  ArenaService gRPC: localhost:443 (LoadBalancer)"
echo "  WireGuard Server: localhost:11820/udp"
echo ""
echo "📖 Useful commands:"
echo "  kubectl get pods --all-namespaces"
echo "  kubectl logs -n control-plane deployment/arenaservice -f"
echo "  kubectl get services -n control-plane"
echo "  /opt/monitor-arena.sh    # Server monitoring"
echo "  /opt/backup-arena.sh     # Configuration backup"
echo ""
echo "🛑 To stop k3s: sudo systemctl stop k3s"
echo "🗑️  To uninstall k3s: /usr/local/bin/k3s-uninstall.sh" 