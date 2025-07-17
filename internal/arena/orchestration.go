package arena

import (
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"time"
)

// ArenaManager orchestrates the full lifecycle of a user arena, coordinating K8s and VPN managers.
type ArenaManager interface {
	CreateArena(publicKey string, vpnIP string, challengeName string, challengeSpec string) (*ArenaConfig, error)
	DestroyArena(publicKey string) error
	GetArenaStatus(publicKey string) (*ArenaStatus, error)
}

// arenaManager is a concrete implementation of ArenaManager.
type arenaManager struct {
	k8s K8sManager
	vpn VPNManager
}

// NewArenaManager constructs a new ArenaManager with the given dependencies.
func NewArenaManager(k8sMgr K8sManager, vpnMgr VPNManager) ArenaManager {
	return &arenaManager{
		k8s: k8sMgr,
		vpn: vpnMgr,
	}
}

func pubKeyToNamespace(pubKey string) string {
	b64 := base64.RawURLEncoding.EncodeToString([]byte(pubKey))
	b64 = strings.ToLower(b64)
	b64 = strings.ReplaceAll(b64, "_", "-")
	return "arena-" + b64
}

// CreateArena provisions a new arena for the user: creates K8s resources and a VPN peer.
func (a *arenaManager) CreateArena(publicKey string, vpnIP string, challengeName string, challengeSpec string) (*ArenaConfig, error) {
	ns := pubKeyToNamespace(publicKey)
	labels := map[string]string{
		"arena/publickey":  publicKey,
		"arena/challenge":  challengeName,
		"arena/created-at": time.Now().UTC().Format(time.RFC3339),
	}

	// 1. Validate the challenge spec before parsing/creating resources
	if err := a.k8s.ValidateSpec(challengeSpec); err != nil {
		return nil, fmt.Errorf("invalid challenge spec: %w", err)
	}

	// 2. Create namespace for the user
	if err := a.k8s.CreateNamespace(ns, labels); err != nil {
		return nil, fmt.Errorf("failed to create namespace: %w", err)
	}

	// 3. Let K8sManager handle all resource creation and endpoint extraction
	k8sEndpoints, err := a.k8s.CreateResourcesFromSpec(ns, challengeSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to create resources from spec: %w", err)
	}

	// Convert []k8s.ServiceEndpoint to []*ServiceEndpoint
	var svcEndpoints []*ServiceEndpoint
	for i := range k8sEndpoints {
		ep := k8sEndpoints[i] // use pointer directly
		svcEndpoints = append(svcEndpoints, &ServiceEndpoint{
			Name:      ep.Name,
			ClusterIp: ep.ClusterIp,
			Ports:     ep.Ports,
		})
	}

	// 4. Apply NetworkPolicy to allow ingress only from the user's VPN IP
	if err := a.k8s.ApplyNetworkPolicy(ns, vpnIP); err != nil {
		return nil, fmt.Errorf("failed to apply network policy: %w", err)
	}

	// 5. Only allow the user's VPN IP as AllowedIP for the peer
	_, ipnet, err := net.ParseCIDR(vpnIP)
	if err != nil {
		return nil, fmt.Errorf("invalid VPN IP: %s", vpnIP)
	}
	allowedIPNets := []net.IPNet{*ipnet}

	// 6. Create VPN peer for the user
	peerCfg, err := a.vpn.CreatePeer(publicKey, allowedIPNets)
	if err != nil {
		return nil, fmt.Errorf("failed to create VPN peer: %w", err)
	}

	// Helper to convert []*ServiceEndpoint to []*ServiceEndpoint (proto)
	convertServiceEndpoints := func(src []*ServiceEndpoint) []*ServiceEndpoint {
		out := make([]*ServiceEndpoint, len(src))
		for i, ep := range src {
			out[i] = &ServiceEndpoint{
				Name:      ep.Name,
				ClusterIp: ep.ClusterIp,
				Ports:     ep.Ports,
			}
		}
		return out
	}

	return &ArenaConfig{
		PublicKey:   peerCfg.PublicKey,
		AllowedIp:   peerCfg.AllowedIP,
		ChallengeId: challengeName,
		Namespace:   ns,
		Endpoints:   convertServiceEndpoints(svcEndpoints),
	}, nil
}

// DestroyArena tears down all resources for the user's arena.
func (a *arenaManager) DestroyArena(publicKey string) error {
	ns := pubKeyToNamespace(publicKey)
	if err := a.vpn.RemovePeer(publicKey); err != nil {
		return fmt.Errorf("failed to remove VPN peer: %w", err)
	}

	if err := a.k8s.DeleteNamespace(ns); err != nil {
		return fmt.Errorf("failed to delete namespace: %w", err)
	}
	return nil
}

// GetArenaStatus returns the current status of the user's arena.
func (a *arenaManager) GetArenaStatus(publicKey string) (*ArenaStatus, error) {
	ns := pubKeyToNamespace(publicKey)

	// Get VPN peer status
	vpnStatus, err := a.vpn.GetPeerStatus(publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get VPN peer status: %w", err)
	}

	// Get K8s namespace status
	k8sStatus, err := a.k8s.GetNamespaceStatus(ns)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace status: %w", err)
	}

	// Determine overall health and collect errors
	var errors []string
	healthy := true

	// Check VPN health
	if vpnStatus == nil {
		errors = append(errors, "VPN peer not found")
		healthy = false
	}

	// Check K8s health
	if k8sStatus != nil {
		if k8sStatus.Phase != "Active" {
			errors = append(errors, fmt.Sprintf("Namespace phase: %s", k8sStatus.Phase))
			healthy = false
		}
		if k8sStatus.PodsFailed > 0 {
			errors = append(errors, fmt.Sprintf("%d pods failed", k8sStatus.PodsFailed))
			healthy = false
		}
		if k8sStatus.ServicesReady < k8sStatus.ServicesTotal {
			errors = append(errors, fmt.Sprintf("%d/%d services ready", k8sStatus.ServicesReady, k8sStatus.ServicesTotal))
			healthy = false
		}
		if !k8sStatus.NetworkPolicyApplied {
			errors = append(errors, "Network policy not applied")
			healthy = false
		}
		// Add any namespace-specific errors
		errors = append(errors, k8sStatus.Errors...)
	} else {
		errors = append(errors, "Namespace not found")
		healthy = false
	}

	return &ArenaStatus{
		PublicKey: publicKey,
		Namespace: ns,
		VpnStatus: vpnStatus,
		K8SStatus: k8sStatus,
		Healthy:   healthy,
		Errors:    errors,
	}, nil
}
