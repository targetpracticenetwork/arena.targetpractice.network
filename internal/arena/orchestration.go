package arena

import (
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"log"
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
	// Hash the public key for consistent length and collision resistance
	hash := sha256.Sum256([]byte(pubKey))

	// Use base32 encoding (URL-safe by default, no padding issues)
	b32 := base32.StdEncoding.EncodeToString(hash[:15])     // 24 chars
	b32 = strings.ToLower(strings.ReplaceAll(b32, "=", "")) // Remove padding, lowercase

	// Truncate to fit K8s namespace limit (63 characters total)
	maxLength := 63 - len("arena-") // Reserve space for "arena-" prefix
	if len(b32) > maxLength {
		b32 = b32[:maxLength]
	}

	return "arena-" + b32
}

// CreateArena provisions a new arena for the user: creates K8s resources and a VPN peer.
func (a *arenaManager) CreateArena(publicKey string, vpnIP string, challengeName string, challengeSpec string) (*ArenaConfig, error) {
	ns := pubKeyToNamespace(publicKey)
	log.Printf("🏗️ Creating arena - Namespace: %s, Challenge: %s", ns, challengeName)

	// Create URL-safe public key hash for labels
	hash := sha256.Sum256([]byte(publicKey))
	keyHash := base32.StdEncoding.EncodeToString(hash[:8]) // 16 chars, no padding
	keyHash = strings.ToLower(strings.ReplaceAll(keyHash, "=", ""))

	labels := map[string]string{
		"arena/publickey":  keyHash,
		"arena/challenge":  challengeName,
		"arena/created-at": time.Now().UTC().Format("20060102T150405Z"),
	}

	// 1. Validate the challenge spec before parsing/creating resources
	log.Printf("🔍 Validating challenge spec for namespace: %s", ns)
	if err := a.k8s.ValidateSpec(challengeSpec); err != nil {
		log.Printf("❌ Challenge spec validation failed: %v", err)
		return nil, fmt.Errorf("invalid challenge spec: %w", err)
	}
	log.Printf("✅ Challenge spec validated successfully")

	// 2. Create namespace for the user
	log.Printf("📦 Creating namespace: %s", ns)
	if err := a.k8s.CreateNamespace(ns, labels); err != nil {
		log.Printf("❌ Namespace creation failed: %v", err)
		return nil, fmt.Errorf("failed to create namespace: %w", err)
	}
	log.Printf("✅ Namespace created: %s", ns)

	// 3. Let K8sManager handle all resource creation and endpoint extraction
	log.Printf("🚀 Creating Kubernetes resources from spec")
	k8sEndpoints, err := a.k8s.CreateResourcesFromSpec(ns, challengeSpec)
	if err != nil {
		log.Printf("❌ Resource creation failed: %v", err)
		return nil, fmt.Errorf("failed to create resources from spec: %w", err)
	}
	log.Printf("✅ Kubernetes resources created - Endpoints: %d", len(k8sEndpoints))

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
	log.Printf("🛡️ Applying network policy for VPN IP: %s", vpnIP)
	if err := a.k8s.ApplyNetworkPolicy(ns, vpnIP); err != nil {
		log.Printf("❌ Network policy application failed: %v", err)
		return nil, fmt.Errorf("failed to apply network policy: %w", err)
	}
	log.Printf("✅ Network policy applied")

	// 5. Only allow the user's VPN IP as AllowedIP for the peer
	_, ipnet, err := net.ParseCIDR(vpnIP)
	if err != nil {
		log.Printf("❌ Invalid VPN IP format: %s", vpnIP)
		return nil, fmt.Errorf("invalid VPN IP: %s", vpnIP)
	}
	allowedIPNets := []net.IPNet{*ipnet}

	// 6. Create VPN peer for the user
	log.Printf("🔐 Creating VPN peer for public key: %s", publicKey)
	peerCfg, err := a.vpn.CreatePeer(publicKey, allowedIPNets)
	if err != nil {
		log.Printf("❌ VPN peer creation failed for public key %s: %v", publicKey, err)
		return nil, fmt.Errorf("failed to create VPN peer: %w", err)
	}
	log.Printf("✅ VPN peer created - Allowed IP: %s", peerCfg.AllowedIP)

	// 7. Get server WireGuard information for client configuration
	log.Printf("🔍 Retrieving server WireGuard configuration...")
	serverPublicKey, err := a.vpn.GetServerPublicKey()
	if err != nil {
		log.Printf("❌ Failed to get server public key: %v", err)
		return nil, fmt.Errorf("failed to get server public key: %w", err)
	}

	serverEndpoint, err := a.vpn.GetServerEndpoint()
	if err != nil {
		log.Printf("❌ Failed to get server endpoint: %v", err)
		return nil, fmt.Errorf("failed to get server endpoint: %w", err)
	}

	allowedIPsForRouting := a.vpn.GetDefaultAllowedIPs()
	log.Printf("✅ Server WireGuard info retrieved - Public Key: %s, Endpoint: %s",
		serverPublicKey, serverEndpoint)

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

	config := &ArenaConfig{
		PublicKey:            peerCfg.PublicKey,
		AllowedIp:            peerCfg.AllowedIP,
		ChallengeId:          challengeName,
		Namespace:            ns,
		Endpoints:            convertServiceEndpoints(svcEndpoints),
		ServerPublicKey:      serverPublicKey,
		ServerEndpoint:       serverEndpoint,
		AllowedIpsForRouting: allowedIPsForRouting,
	}

	log.Printf("🎉 Arena creation completed - Namespace: %s, Endpoints: %d", ns, len(config.Endpoints))
	return config, nil
}

// DestroyArena tears down all resources for the user's arena.
func (a *arenaManager) DestroyArena(publicKey string) error {
	ns := pubKeyToNamespace(publicKey)
	log.Printf("🗑️ Destroying arena - Namespace: %s", ns)

	log.Printf("🔐 Removing VPN peer for public key: %s", publicKey)
	if err := a.vpn.RemovePeer(publicKey); err != nil {
		log.Printf("❌ VPN peer removal failed for public key %s: %v", publicKey, err)
		return fmt.Errorf("failed to remove VPN peer: %w", err)
	}
	log.Printf("✅ VPN peer removed")

	log.Printf("📦 Deleting namespace: %s", ns)
	if err := a.k8s.DeleteNamespace(ns); err != nil {
		log.Printf("❌ Namespace deletion failed: %v", err)
		return fmt.Errorf("failed to delete namespace: %w", err)
	}
	log.Printf("✅ Namespace deleted: %s", ns)

	log.Printf("🎉 Arena destruction completed - Namespace: %s", ns)
	return nil
}

// GetArenaStatus returns the current status of the user's arena.
func (a *arenaManager) GetArenaStatus(publicKey string) (*ArenaStatus, error) {
	ns := pubKeyToNamespace(publicKey)
	log.Printf("📊 Getting arena status - Namespace: %s", ns)

	// Get VPN peer status
	log.Printf("🔐 Getting VPN peer status for public key: %s", publicKey)
	vpnStatus, err := a.vpn.GetPeerStatus(publicKey)
	if err != nil {
		log.Printf("❌ VPN peer status failed for public key %s: %v", publicKey, err)
		return nil, fmt.Errorf("failed to get VPN peer status: %w", err)
	}
	log.Printf("✅ VPN peer status retrieved")

	// Get K8s namespace status
	log.Printf("📦 Getting Kubernetes namespace status: %s", ns)
	k8sStatus, err := a.k8s.GetNamespaceStatus(ns)
	if err != nil {
		log.Printf("❌ Kubernetes namespace status failed: %v", err)
		return nil, fmt.Errorf("failed to get namespace status: %w", err)
	}
	log.Printf("✅ Kubernetes namespace status retrieved - Pods: %d, Services: %d",
		k8sStatus.GetPodCount(), k8sStatus.GetServiceCount())

	// Determine overall health and collect errors
	var errors []string
	healthy := true

	// Check VPN health
	if vpnStatus == nil {
		errors = append(errors, "VPN peer not found")
		healthy = false
		log.Printf("⚠️ VPN peer not found")
	}

	// Check K8s health
	if k8sStatus != nil {
		if k8sStatus.Phase != "Active" {
			errors = append(errors, fmt.Sprintf("Namespace phase: %s", k8sStatus.Phase))
			healthy = false
			log.Printf("⚠️ Namespace phase: %s", k8sStatus.Phase)
		}
		if k8sStatus.PodsFailed > 0 {
			errors = append(errors, fmt.Sprintf("%d pods failed", k8sStatus.PodsFailed))
			healthy = false
			log.Printf("⚠️ %d pods failed", k8sStatus.PodsFailed)
		}
		if k8sStatus.ServicesReady < k8sStatus.ServicesTotal {
			errors = append(errors, fmt.Sprintf("%d/%d services ready", k8sStatus.ServicesReady, k8sStatus.ServicesTotal))
			healthy = false
			log.Printf("⚠️ %d/%d services ready", k8sStatus.ServicesReady, k8sStatus.ServicesTotal)
		}
		if !k8sStatus.NetworkPolicyApplied {
			errors = append(errors, "Network policy not applied")
			healthy = false
			log.Printf("⚠️ Network policy not applied")
		}
		// Add any namespace-specific errors
		errors = append(errors, k8sStatus.Errors...)
	} else {
		errors = append(errors, "Namespace not found")
		healthy = false
		log.Printf("⚠️ Namespace not found")
	}

	status := &ArenaStatus{
		PublicKey: publicKey,
		Namespace: ns,
		VpnStatus: vpnStatus,
		K8SStatus: k8sStatus,
		Healthy:   healthy,
		Errors:    errors,
	}

	healthStatus := "✅"
	if !healthy {
		healthStatus = "❌"
	}
	log.Printf("📊 Arena status completed - Namespace: %s, Healthy: %s, Errors: %d",
		ns, healthStatus, len(errors))

	return status, nil
}
