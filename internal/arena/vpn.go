package arena

import (
	"net"

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
	CreatePeer(pubKey string, allowedIPs []net.IPNet) (*PeerConfig, error)
	RemovePeer(pubKey string) error
	GetPeerStatus(pubKey string) (*PeerStatus, error)
	CheckDevice() error
}

// WGManager is a concrete implementation of VPNManager using wgctrl.
type WGManager struct {
	InterfaceName string
}

// CreatePeer adds a peer to the WireGuard interface using the provided public key and allowed IPs.
func (w *WGManager) CreatePeer(pubKey string, allowedIPs []net.IPNet) (*PeerConfig, error) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, err
	}
	defer client.Close()

	key, err := wgtypes.ParseKey(pubKey)
	if err != nil {
		return nil, err
	}

	peerCfg := wgtypes.PeerConfig{
		PublicKey:         key,
		AllowedIPs:        allowedIPs,
		ReplaceAllowedIPs: true,
	}

	cfg := wgtypes.Config{
		Peers: []wgtypes.PeerConfig{peerCfg},
	}

	err = client.ConfigureDevice(w.InterfaceName, cfg)
	if err != nil {
		return nil, err
	}

	// Only return the first allowed IP for encapsulation
	allowedIP := ""
	if len(allowedIPs) > 0 {
		allowedIP = allowedIPs[0].String()
	}

	return &PeerConfig{
		PublicKey: key.String(),
		AllowedIP: allowedIP,
	}, nil
}

// RemovePeer removes a peer from the WireGuard interface using the provided public key.
func (w *WGManager) RemovePeer(pubKey string) error {
	client, err := wgctrl.New()
	if err != nil {
		return err
	}
	defer client.Close()

	key, err := wgtypes.ParseKey(pubKey)
	if err != nil {
		return err
	}

	peerCfg := wgtypes.PeerConfig{
		PublicKey: key,
		Remove:    true,
	}

	cfg := wgtypes.Config{
		Peers: []wgtypes.PeerConfig{peerCfg},
	}

	return client.ConfigureDevice(w.InterfaceName, cfg)
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
