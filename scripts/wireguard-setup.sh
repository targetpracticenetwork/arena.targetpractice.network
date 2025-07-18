#!/bin/bash

set -e

# WireGuard setup script for ArenaService
# This script initializes the WireGuard interface and sets up basic configuration

WIREGUARD_INTERFACE=${WIREGUARD_INTERFACE:-wg0}
WIREGUARD_PRIVATE_KEY=${WIREGUARD_PRIVATE_KEY:-}
WIREGUARD_PUBLIC_KEY=${WIREGUARD_PUBLIC_KEY:-}
WIREGUARD_ADDRESS=${WIREGUARD_ADDRESS:-10.0.0.1/24}
WIREGUARD_PORT=${WIREGUARD_PORT:-11820}

echo "Setting up WireGuard interface: $WIREGUARD_INTERFACE"

# Check if WireGuard module is loaded
if ! lsmod | grep -q wireguard; then
    echo "Loading WireGuard kernel module..."
    modprobe wireguard || {
        echo "Failed to load WireGuard kernel module. Make sure WireGuard is installed."
        exit 1
    }
fi

# Generate keys if not provided
if [ -z "$WIREGUARD_PRIVATE_KEY" ]; then
    echo "Generating WireGuard private key..."
    WIREGUARD_PRIVATE_KEY=$(wg genkey)
    echo "Generated private key: $WIREGUARD_PRIVATE_KEY"
fi

if [ -z "$WIREGUARD_PUBLIC_KEY" ]; then
    echo "Generating WireGuard public key..."
    WIREGUARD_PUBLIC_KEY=$(echo "$WIREGUARD_PRIVATE_KEY" | wg pubkey)
    echo "Generated public key: $WIREGUARD_PUBLIC_KEY"
fi

# Create WireGuard configuration directory
mkdir -p /etc/wireguard

# Create WireGuard configuration file
cat > /etc/wireguard/$WIREGUARD_INTERFACE.conf << EOF
[Interface]
PrivateKey = $WIREGUARD_PRIVATE_KEY
Address = $WIREGUARD_ADDRESS
ListenPort = $WIREGUARD_PORT
SaveConfig = true

# Enable IP forwarding
PostUp = sysctl -w net.ipv4.ip_forward=1
PostUp = sysctl -w net.ipv6.conf.all.forwarding=1

# Set up iptables rules for NAT
PostUp = iptables -A FORWARD -i $WIREGUARD_INTERFACE -j ACCEPT
PostUp = iptables -A FORWARD -o $WIREGUARD_INTERFACE -j ACCEPT
PostUp = iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE

# Clean up iptables rules on interface down
PostDown = iptables -D FORWARD -i $WIREGUARD_INTERFACE -j ACCEPT
PostDown = iptables -D FORWARD -o $WIREGUARD_INTERFACE -j ACCEPT
PostDown = iptables -t nat -D POSTROUTING -o eth0 -j MASQUERADE
EOF

# Set proper permissions
chmod 600 /etc/wireguard/$WIREGUARD_INTERFACE.conf

# Create the WireGuard interface
echo "Creating WireGuard interface..."
ip link add dev $WIREGUARD_INTERFACE type wireguard

# Configure the interface
echo "Configuring WireGuard interface..."
wg setconf $WIREGUARD_INTERFACE /etc/wireguard/$WIREGUARD_INTERFACE.conf

# Bring the interface up
echo "Bringing WireGuard interface up..."
ip link set dev $WIREGUARD_INTERFACE up

# Add route for WireGuard network
echo "Adding route for WireGuard network..."
ip route add $WIREGUARD_ADDRESS dev $WIREGUARD_INTERFACE

# Verify interface is up
if ip link show $WIREGUARD_INTERFACE | grep -q "state UP"; then
    echo "WireGuard interface $WIREGUARD_INTERFACE is up and running"
    echo "Public key: $WIREGUARD_PUBLIC_KEY"
    echo "Interface address: $WIREGUARD_ADDRESS"
    echo "Listen port: $WIREGUARD_PORT"
else
    echo "Failed to bring up WireGuard interface"
    exit 1
fi

# Export the public key for ArenaService to use
export WIREGUARD_PUBLIC_KEY
echo "$WIREGUARD_PUBLIC_KEY" > /tmp/wireguard_public_key

echo "WireGuard setup completed successfully" 