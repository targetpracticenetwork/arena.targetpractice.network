package arena

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// ArenaManager orchestrates the full lifecycle of a user arena, coordinating K8s and VPN managers.
type ArenaManager interface {
	CreateArena(ctx context.Context, publicKey string, vpnIP string, challengeName string, challengeSpec string) (*ArenaConfig, error)
	DestroyArena(ctx context.Context, vpnIP string) error
	GetArenaStatus(ctx context.Context, vpnIP string) (*ArenaStatus, error)
}

// arenaManager is a concrete implementation of ArenaManager.
type arenaManager struct {
	k8s                     K8sManager
	vpn                     VPNManager
	creationTimeouts        sync.Map // vpnIP -> *creationTimeout
	creationTimeoutDuration time.Duration
}

type creationTimeout struct {
	timer     *time.Timer
	vpnIP     string
	startTime time.Time
}

// NewArenaManager constructs a new ArenaManager with the given dependencies.
func NewArenaManager(k8sMgr K8sManager, vpnMgr VPNManager) ArenaManager {
	return &arenaManager{
		k8s:                     k8sMgr,
		vpn:                     vpnMgr,
		creationTimeoutDuration: 5 * time.Minute, // default 5 minutes
	}
}

// NewArenaManagerWithTimeout constructs a new ArenaManager with a custom timeout duration.
func NewArenaManagerWithTimeout(k8sMgr K8sManager, vpnMgr VPNManager, timeout time.Duration) ArenaManager {
	return &arenaManager{
		k8s:                     k8sMgr,
		vpn:                     vpnMgr,
		creationTimeoutDuration: timeout,
	}
}

// vpnIPToNamespace creates a DNS-compliant namespace name from a VPN IP address.
// Example: 10.6.1.23 becomes arena-10-6-1-23
func vpnIPToNamespace(vpnIP string) string {
	// Parse the VPN IP to extract just the IP address (without CIDR)
	ip := vpnIP
	if strings.Contains(vpnIP, "/") {
		ip = strings.Split(vpnIP, "/")[0]
	}

	// Replace dots with dashes to make it DNS-compliant
	ipPart := strings.ReplaceAll(ip, ".", "-")

	// Create namespace name with arena- prefix
	namespaceName := "arena-" + ipPart

	// Ensure it meets Kubernetes namespace requirements (max 63 characters, DNS-compliant)
	if len(namespaceName) > 63 {
		// This should rarely happen with IPv4 addresses, but truncate if needed
		namespaceName = namespaceName[:63]
	}

	return namespaceName
}

// CreateArena provisions a new arena for the user: creates K8s resources and a VPN peer.
func (a *arenaManager) CreateArena(ctx context.Context, publicKey string, vpnIP string, challengeName string, challengeSpec string) (*ArenaConfig, error) {
	ns := vpnIPToNamespace(vpnIP)
	log.Printf("🏗️ Creating arena - Namespace: %s, Challenge: %s", ns, challengeName)

	// Start creation timeout - this will clean up if creation fails or hangs
	timeout := &creationTimeout{
		vpnIP:     vpnIP,
		startTime: time.Now(),
	}

	timeout.timer = time.AfterFunc(a.creationTimeoutDuration, func() {
		log.Printf("⏰ Arena creation timeout reached for VPN IP: %s, cleaning up potentially failed arena", vpnIP)

		// Use a new context with its own timeout for cleanup
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		// Check if arena was successfully created by checking its status
		status, err := a.GetArenaStatus(cleanupCtx, vpnIP)
		if err != nil || status == nil || !status.Healthy {
			log.Printf("🧹 Arena appears to have failed creation, destroying resources for VPN IP: %s", vpnIP)
			if destroyErr := a.DestroyArena(cleanupCtx, vpnIP); destroyErr != nil {
				log.Printf("❌ Failed to destroy potentially failed arena: %v", destroyErr)
			} else {
				log.Printf("✅ Successfully cleaned up failed arena for VPN IP: %s", vpnIP)
			}
		} else {
			log.Printf("✅ Arena is healthy, skipping cleanup for VPN IP: %s", vpnIP)
		}

		// Remove from map
		a.creationTimeouts.Delete(vpnIP)
	})

	a.creationTimeouts.Store(vpnIP, timeout)

	_, vpnIPNet, err := net.ParseCIDR(vpnIP)
	if err != nil {
		log.Printf("❌ Invalid VPN IP format: %s", vpnIP)
		a.cancelCreationTimeout(vpnIP)
		return nil, fmt.Errorf("invalid VPN IP: %s", vpnIP)
	}

	labels := map[string]string{
		"arena/challenge":  challengeName,
		"arena/created-at": time.Now().UTC().Format("20060102T150405Z"),
		"arena/namespace":  ns,
		"arena/creating":   "true", // Mark as still creating
	}

	// 1. Validate the challenge spec before parsing/creating resources
	log.Printf("🔍 Validating challenge spec for namespace: %s", ns)
	if err := a.k8s.ValidateSpec(ctx, challengeSpec); err != nil {
		log.Printf("❌ Challenge spec validation failed: %v", err)
		a.cancelCreationTimeout(vpnIP)
		return nil, fmt.Errorf("invalid challenge spec: %w", err)
	}
	log.Printf("✅ Challenge spec validated successfully")

	// 2. Create namespace for the user
	log.Printf("📦 Creating namespace: %s", ns)
	if err := a.k8s.CreateNamespace(ctx, ns, labels); err != nil {
		log.Printf("❌ Namespace creation failed: %v", err)
		a.cancelCreationTimeout(vpnIP)
		return nil, fmt.Errorf("failed to create namespace: %w", err)
	}
	log.Printf("✅ Namespace created: %s", ns)

	// 3. Let K8sManager handle all resource creation and endpoint extraction
	log.Printf("🚀 Creating Kubernetes resources from spec")
	k8sEndpoints, err := a.k8s.CreateResourcesFromSpec(ctx, ns, challengeSpec)
	if err != nil {
		log.Printf("❌ Resource creation failed: %v", err)
		a.cancelCreationTimeout(vpnIP)
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

	// 4. Apply NetworkPolicy to allow ingress from VPN IP and specific service IPs
	log.Printf("🛡️ Applying network policy for VPN IP: %s", vpnIP)

	// Build service IPs list for network policy
	var serviceIPs []string
	log.Printf("🔍 Building service IPs list from %d endpoints", len(svcEndpoints))
	for _, endpoint := range svcEndpoints {
		log.Printf("🔍 Processing endpoint: %s -> %s", endpoint.Name, endpoint.ClusterIp)
		if endpoint.ClusterIp != "" {
			serviceIP := endpoint.ClusterIp + "/32"
			serviceIPs = append(serviceIPs, serviceIP)
			log.Printf("🔧 Added service IP to network policy: %s", serviceIP)
		} else {
			log.Printf("⚠️ Endpoint %s has empty cluster IP", endpoint.Name)
		}
	}
	log.Printf("🔧 Total service IPs for network policy: %d", len(serviceIPs))

	if err := a.k8s.ApplyNetworkPolicy(ctx, ns, vpnIP, serviceIPs); err != nil {
		log.Printf("❌ Network policy application failed: %v", err)
		a.cancelCreationTimeout(vpnIP)
		return nil, fmt.Errorf("failed to apply network policy: %w", err)
	}
	log.Printf("✅ Network policy applied with %d service IPs", len(serviceIPs))

	// 6. Build allowed destination IPs (service IPs only)
	var allowedDestinations []net.IPNet
	for _, endpoint := range svcEndpoints {
		if endpoint.ClusterIp != "" {
			serviceIP := endpoint.ClusterIp + "/32"
			_, serviceIPNet, err := net.ParseCIDR(serviceIP)
			if err != nil {
				log.Printf("❌ Invalid service IP format: %s", serviceIP)
				a.cancelCreationTimeout(vpnIP)
				return nil, fmt.Errorf("invalid service IP: %s", serviceIP)
			}
			allowedDestinations = append(allowedDestinations, *serviceIPNet)
			log.Printf("🔧 Adding service IP to allowed destinations: %s", serviceIP)
		}
	}

	log.Printf("🔧 Configuring peer - VPN IP: %s, Allowed destinations: %d", vpnIPNet.String(), len(allowedDestinations))

	// 7. Create VPN peer with proper source/destination separation
	log.Printf("🔐 Creating VPN peer for public key: %s", publicKey)
	peerCfg, err := a.vpn.CreatePeer(ctx, publicKey, *vpnIPNet, allowedDestinations)
	if err != nil {
		log.Printf("❌ VPN peer creation failed for public key %s: %v", publicKey, err)
		a.cancelCreationTimeout(vpnIP)
		return nil, fmt.Errorf("failed to create VPN peer: %w", err)
	}
	log.Printf("✅ VPN peer created with destination filtering - VPN IP: %s, Destinations: %d", peerCfg.AllowedIP, len(allowedDestinations))

	// 8. Get server WireGuard information for client configuration
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

	// Build allowed IPs for routing: VPN subnet + specific service IPs
	allowedIPsForRouting := a.vpn.GetDefaultAllowedIPs() // Default VPN subnet

	// Add specific service IPs as /32 CIDRs
	for _, endpoint := range svcEndpoints {
		if endpoint.ClusterIp != "" {
			serviceIP := endpoint.ClusterIp + "/32"
			allowedIPsForRouting = append(allowedIPsForRouting, serviceIP)
			log.Printf("🔧 Adding service IP to routing: %s", serviceIP)
		}
	}

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

	// Mark creation as complete and cancel timeout
	a.cancelCreationTimeout(vpnIP)

	log.Printf("🎉 Arena creation completed - Namespace: %s, Endpoints: %d", ns, len(config.Endpoints))
	return config, nil
}

// cancelCreationTimeout cancels any pending creation timeout for the given VPN IP
func (a *arenaManager) cancelCreationTimeout(vpnIP string) {
	if timeout, ok := a.creationTimeouts.LoadAndDelete(vpnIP); ok {
		timeout.(*creationTimeout).timer.Stop()
		log.Printf("⏹️ Cancelled creation timeout for VPN IP: %s", vpnIP)
	}
}

// DestroyArena tears down all resources for the user's arena.
func (a *arenaManager) DestroyArena(ctx context.Context, vpnIP string) error {
	ns := vpnIPToNamespace(vpnIP)
	log.Printf("🗑️ Destroying arena - Namespace: %s", ns)

	// Cancel any pending creation timeout
	a.cancelCreationTimeout(vpnIP)

	log.Printf("🔐 Removing VPN peer for VPN IP: %s", vpnIP)
	if err := a.vpn.RemovePeerByIP(ctx, vpnIP); err != nil {
		log.Printf("❌ VPN peer removal failed for VPN IP %s: %v", vpnIP, err)
	} else {
		log.Printf("✅ VPN peer removed")
	}

	log.Printf("📦 Deleting namespace: %s", ns)
	if err := a.k8s.DeleteNamespace(ctx, ns); err != nil {
		log.Printf("❌ Namespace deletion failed: %v", err)
	} else {
		log.Printf("✅ Namespace deleted: %s", ns)
	}

	log.Printf("🎉 Arena destruction completed - Namespace: %s", ns)
	return nil
}

// GetArenaStatus returns the current status of the user's arena.
func (a *arenaManager) GetArenaStatus(ctx context.Context, vpnIP string) (*ArenaStatus, error) {
	ns := vpnIPToNamespace(vpnIP)
	log.Printf("📊 Getting arena status - Namespace: %s", ns)

	// Get VPN peer status
	log.Printf("🔐 Getting VPN peer status for VPN IP: %s", vpnIP)
	vpnStatus, err := a.vpn.GetPeerStatusByIP(ctx, vpnIP)
	if err != nil {
		log.Printf("❌ VPN peer status failed for VPN IP %s: %v", vpnIP, err)
		return nil, fmt.Errorf("failed to get VPN peer status: %w", err)
	}
	log.Printf("✅ VPN peer status retrieved")

	// Get K8s namespace status
	log.Printf("📦 Getting Kubernetes namespace status: %s", ns)
	k8sStatus, err := a.k8s.GetNamespaceStatus(ctx, ns)
	if err != nil {
		log.Printf("❌ Kubernetes namespace status failed: %v", err)
		return nil, fmt.Errorf("failed to get namespace status: %w", err)
	}
	log.Printf("✅ Kubernetes namespace status retrieved - Pods: %d, Services: %d",
		k8sStatus.GetPodCount(), k8sStatus.GetServiceCount())

	// Simplified health check: VPN connected and pods > 0
	var errors []string
	healthy := true

	// Check VPN health
	if vpnStatus == nil {
		errors = append(errors, "VPN peer not found")
		healthy = false
		log.Printf("⚠️ VPN peer not found")
	}

	// Check if pods are running
	if k8sStatus != nil {
		if k8sStatus.PodCount == 0 {
			errors = append(errors, "No pods running")
			healthy = false
			log.Printf("⚠️ No pods running")
		}
		// Add any namespace-specific errors
		errors = append(errors, k8sStatus.Errors...)
	} else {
		errors = append(errors, "Namespace not found")
		healthy = false
		log.Printf("⚠️ Namespace not found")
	}

	// Get server public key for client configuration
	serverPublicKey, err := a.vpn.GetServerPublicKey()
	if err != nil {
		log.Printf("❌ Failed to get server public key: %v", err)
		serverPublicKey = "unknown"
	}

	status := &ArenaStatus{
		PublicKey: serverPublicKey, // Use server's public key for client configuration
		Namespace: ns,
		VpnStatus: vpnStatus,
		K8SStatus: k8sStatus,
		Healthy:   true,
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

// Shutdown gracefully shuts down the arena manager, cancelling all pending timeouts
func (a *arenaManager) Shutdown(ctx context.Context) error {
	log.Printf("🛑 Shutting down arena manager, cancelling all pending timeouts")
	
	count := 0
	a.creationTimeouts.Range(func(key, value interface{}) bool {
		if timeout, ok := value.(*creationTimeout); ok {
			timeout.timer.Stop()
			count++
		}
		return true
	})
	
	log.Printf("✅ Cancelled %d pending creation timeouts", count)
	return nil
}
