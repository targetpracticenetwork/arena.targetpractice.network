package arena

import (
	"fmt"
	"log"
	"net"
	"os"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// PeerConfig is a project-level struct for returning peer info from VPNManager.
type PeerConfig struct {
	PublicKey string
	AllowedIP string
}

// VPNManager defines the interface for managing VPN peers.
type VPNManager interface {
	CreatePeer(pubKey string, vpnIP net.IPNet, allowedDestinations []net.IPNet) (*PeerConfig, error)
	RemovePeer(pubKey string) error
	GetPeerStatus(pubKey string) (*PeerStatus, error)
	CheckDevice() error
	GetServerPublicKey() (string, error)
	GetServerEndpoint() (string, error)
	GetDefaultAllowedIPs() []string

	// CIDR configuration methods
	GetVPNSubnet() string
	GetServiceCIDR() string
}

// WGManager is a concrete implementation of VPNManager using wgctrl.
type WGManager struct {
	InterfaceName   string
	ServerPublicKey string
	ServerEndpoint  string
	VPNSubnet       string // VPN subnet CIDR
	ServiceCIDR     string // Kubernetes service network CIDR
}

// NewWGManager creates a new WGManager and initializes server configuration at startup.
func NewWGManager(interfaceName string, vpnSubnet string, serviceCIDR string) (*WGManager, error) {
	mgr := &WGManager{
		InterfaceName: interfaceName,
		VPNSubnet:     vpnSubnet,
		ServiceCIDR:   serviceCIDR,
	}

	// Initialize server configuration at startup
	if err := mgr.initializeServerConfig(); err != nil {
		return nil, fmt.Errorf("failed to initialize WireGuard server config: %w", err)
	}

	return mgr, nil
}

// initializeServerConfig retrieves and caches server configuration at startup.
func (w *WGManager) initializeServerConfig() error {
	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("failed to create WireGuard client: %w", err)
	}
	defer client.Close()

	device, err := client.Device(w.InterfaceName)
	if err != nil {
		return fmt.Errorf("failed to get WireGuard device %s: %w", w.InterfaceName, err)
	}

	// Cache server public key
	w.ServerPublicKey = device.PublicKey.String()

	// Get server hostname/IP from environment variable
	serverHost := os.Getenv("WIREGUARD_SERVER_HOST")
	if serverHost == "" {
		serverHost = "localhost" // fallback for development
	}

	// Cache server endpoint with port
	port := 11820 // default port
	if device.ListenPort != 0 {
		port = int(device.ListenPort)
	}
	w.ServerEndpoint = fmt.Sprintf("%s:%d", serverHost, port)

	log.Printf("🔧 WireGuard server config initialized - Public Key: %s, Endpoint: %s",
		w.ServerPublicKey, w.ServerEndpoint)

	return nil
}

// CreatePeer adds a peer to the WireGuard interface using the provided public key and destination IPs.
func (w *WGManager) CreatePeer(pubKey string, vpnIP net.IPNet, allowedDestinations []net.IPNet) (*PeerConfig, error) {
	log.Printf("🔐 Creating WireGuard peer - Interface: %s, Public Key: %s, VPN IP: %s",
		w.InterfaceName, pubKey, vpnIP.String())

	client, err := wgctrl.New()
	if err != nil {
		log.Printf("❌ Failed to create WireGuard client: %v", err)
		return nil, err
	}
	defer client.Close()

	key, err := wgtypes.ParseKey(pubKey)
	if err != nil {
		log.Printf("❌ Failed to parse WireGuard public key: %v", err)
		return nil, err
	}

	// Only allow the peer's VPN IP as source (server-side source validation)
	peerCfg := wgtypes.PeerConfig{
		PublicKey:         key,
		AllowedIPs:        []net.IPNet{vpnIP},
		ReplaceAllowedIPs: true,
	}

	cfg := wgtypes.Config{
		Peers: []wgtypes.PeerConfig{peerCfg},
	}

	log.Printf("🔧 Configuring WireGuard device with source IP: %s", vpnIP.String())
	err = client.ConfigureDevice(w.InterfaceName, cfg)
	if err != nil {
		log.Printf("❌ Failed to configure WireGuard device: %v", err)
		return nil, err
	}

	log.Printf("✅ WireGuard peer created - VPN IP: %s, Destinations: %d",
		vpnIP.String(), len(allowedDestinations))

	return &PeerConfig{
		PublicKey: key.String(),
		AllowedIP: vpnIP.String(),
	}, nil
}

// RemovePeer removes a peer from the WireGuard interface using the provided public key.
func (w *WGManager) RemovePeer(pubKey string) error {
	log.Printf("🗑️ Removing WireGuard peer - Interface: %s, Public Key: %s", w.InterfaceName, pubKey)

	client, err := wgctrl.New()
	if err != nil {
		log.Printf("❌ Failed to create WireGuard client: %v", err)
		return err
	}
	defer client.Close()

	key, err := wgtypes.ParseKey(pubKey)
	if err != nil {
		log.Printf("❌ Failed to parse WireGuard public key: %v", err)
		return err
	}

	// Remove the peer by setting an empty allowed IPs list
	peerCfg := wgtypes.PeerConfig{
		PublicKey:         key,
		AllowedIPs:        []net.IPNet{},
		ReplaceAllowedIPs: true,
		Remove:            true,
	}

	cfg := wgtypes.Config{
		Peers: []wgtypes.PeerConfig{peerCfg},
	}

	log.Printf("🔧 Removing peer from WireGuard device: %s", w.InterfaceName)
	err = client.ConfigureDevice(w.InterfaceName, cfg)
	if err != nil {
		log.Printf("❌ Failed to remove WireGuard peer: %v", err)
		return err
	}

	log.Printf("✅ WireGuard peer removed successfully")
	return nil
}

// GetPeerStatus returns information about the peer with the given public key.
func (w *WGManager) GetPeerStatus(pubKey string) (*PeerStatus, error) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, err
	}
	defer client.Close()

	key, err := wgtypes.ParseKey(pubKey)
	if err != nil {
		return nil, err
	}

	device, err := client.Device(w.InterfaceName)
	if err != nil {
		return nil, err
	}

	for _, peer := range device.Peers {
		if peer.PublicKey == key {
			allowedIPs := make([]string, len(peer.AllowedIPs))
			for i, ipnet := range peer.AllowedIPs {
				allowedIPs[i] = ipnet.String()
			}
			status := PeerStatus{
				PublicKey:     peer.PublicKey.String(),
				Endpoint:      "",
				AllowedIps:    allowedIPs,
				LastHandshake: peer.LastHandshakeTime.Unix(),
				RxBytes:       int64(peer.ReceiveBytes),
				TxBytes:       int64(peer.TransmitBytes),
			}
			if peer.Endpoint != nil {
				status.Endpoint = peer.Endpoint.String()
			}
			return &status, nil
		}
	}
	return nil, nil // Not found
}

// CheckDevice verifies that the WireGuard device is accessible and working.
func (w *WGManager) CheckDevice() error {
	client, err := wgctrl.New()
	if err != nil {
		return err
	}
	defer client.Close()

	_, err = client.Device(w.InterfaceName)
	return err
}

// GetServerPublicKey returns the server's WireGuard public key (cached at startup).
func (w *WGManager) GetServerPublicKey() (string, error) {
	return w.ServerPublicKey, nil
}

// GetServerEndpoint returns the server's endpoint for client configuration (cached at startup).
func (w *WGManager) GetServerEndpoint() (string, error) {
	return w.ServerEndpoint, nil
}

// GetDefaultAllowedIPs returns the default allowed IPs for client routing.
func (w *WGManager) GetDefaultAllowedIPs() []string {
	return []string{w.VPNSubnet, w.ServiceCIDR}
}

// GetVPNSubnet returns the VPN subnet CIDR.
func (w *WGManager) GetVPNSubnet() string {
	return w.VPNSubnet
}

// GetServiceCIDR returns the Kubernetes service network CIDR.
func (w *WGManager) GetServiceCIDR() string {
	return w.ServiceCIDR
}
