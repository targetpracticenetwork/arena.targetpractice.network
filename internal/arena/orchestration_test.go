package arena

import (
	"errors"
	"net"
	"testing"

	v1 "k8s.io/api/core/v1"
)

type mockK8sManager struct {
	CreateNamespaceCalled         bool
	DeleteNamespaceCalled         bool
	ValidateSpecCalled            bool
	CreateResourcesFromSpecCalled bool
	ApplyNetworkPolicyCalled      bool
	GetNamespaceStatusCalled      bool

	CreateNamespaceErr         error
	DeleteNamespaceErr         error
	ValidateSpecErr            error
	CreateResourcesFromSpecErr error
	ApplyNetworkPolicyErr      error
	GetNamespaceStatusErr      error
	Resources                  []*ServiceEndpoint
	NamespaceStatus            *NamespaceStatus
}

func (m *mockK8sManager) CreateNamespace(name string, labels map[string]string) error {
	m.CreateNamespaceCalled = true
	return m.CreateNamespaceErr
}
func (m *mockK8sManager) DeleteNamespace(name string) error {
	m.DeleteNamespaceCalled = true
	return m.DeleteNamespaceErr
}
func (m *mockK8sManager) CreatePods(namespace string, pods []*v1.Pod) error             { return nil }
func (m *mockK8sManager) DeletePods(namespace string, podNames []string) error          { return nil }
func (m *mockK8sManager) CreateServices(namespace string, services []*v1.Service) error { return nil }
func (m *mockK8sManager) DeleteServices(namespace string, serviceNames []string) error  { return nil }
func (m *mockK8sManager) ApplyNetworkPolicy(namespace string, vpnIP string, serviceIPs []string) error {
	m.ApplyNetworkPolicyCalled = true
	return m.ApplyNetworkPolicyErr
}
func (m *mockK8sManager) ApplyResourceQuota(namespace string, quota *ResourceUsage) error { return nil }
func (m *mockK8sManager) GetNamespaceStatus(name string) (*NamespaceStatus, error) {
	m.GetNamespaceStatusCalled = true
	if m.GetNamespaceStatusErr != nil {
		return nil, m.GetNamespaceStatusErr
	}
	return m.NamespaceStatus, nil
}
func (m *mockK8sManager) ValidateSpec(challengeSpec string) error {
	m.ValidateSpecCalled = true
	return m.ValidateSpecErr
}
func (m *mockK8sManager) CreateResourcesFromSpec(namespace, challengeSpec string) ([]*ServiceEndpoint, error) {
	m.CreateResourcesFromSpecCalled = true
	return m.Resources, m.CreateResourcesFromSpecErr
}

type mockVPNManager struct {
	CreatePeerCalled    bool
	RemovePeerCalled    bool
	GetPeerStatusCalled bool

	CreatePeerErr    error
	RemovePeerErr    error
	GetPeerStatusErr error
	PeerCfg          *PeerConfig
	PeerStatus       *PeerStatus

	// New fields for server info
	ServerPublicKey   string
	ServerEndpoint    string
	DefaultAllowedIPs []string
}

func (m *mockVPNManager) CreatePeer(pubKey string, allowedIPs []net.IPNet) (*PeerConfig, error) {
	m.CreatePeerCalled = true
	return m.PeerCfg, m.CreatePeerErr
}
func (m *mockVPNManager) RemovePeer(pubKey string) error {
	m.RemovePeerCalled = true
	return m.RemovePeerErr
}
func (m *mockVPNManager) GetPeerStatus(pubKey string) (*PeerStatus, error) {
	m.GetPeerStatusCalled = true
	return m.PeerStatus, m.GetPeerStatusErr
}
func (m *mockVPNManager) CheckDevice() error { return nil }
func (m *mockVPNManager) GetServerPublicKey() (string, error) {
	if m.ServerPublicKey == "" {
		return "server-public-key", nil
	}
	return m.ServerPublicKey, nil
}
func (m *mockVPNManager) GetServerEndpoint() (string, error) {
	if m.ServerEndpoint == "" {
		return "arena-dev.targetpractice.network:11820", nil
	}
	return m.ServerEndpoint, nil
}
func (m *mockVPNManager) GetDefaultAllowedIPs() []string {
	if m.DefaultAllowedIPs == nil {
		return []string{"10.0.0.0/24"}
	}
	return m.DefaultAllowedIPs
}

func TestArenaManager_CreateArena_Success(t *testing.T) {
	k8s := &mockK8sManager{
		Resources: []*ServiceEndpoint{{Name: "svc", ClusterIp: "10.0.0.1", Ports: []int32{80}}},
	}
	vpn := &mockVPNManager{
		PeerCfg: &PeerConfig{PublicKey: "pk", AllowedIP: "10.0.0.2/32"},
	}
	mgr := NewArenaManager(k8s, vpn)
	cfg, err := mgr.CreateArena("pk", "10.0.0.2/32", "test-challenge", "spec")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PublicKey != "pk" || cfg.AllowedIp != "10.0.0.2/32" {
		t.Errorf("unexpected config: %+v", &cfg)
	}
	// Verify new WireGuard server information fields
	if cfg.ServerPublicKey != "server-public-key" {
		t.Errorf("expected server public key 'server-public-key', got '%s'", cfg.ServerPublicKey)
	}
	if cfg.ServerEndpoint != "arena-dev.targetpractice.network:11820" {
		t.Errorf("expected server endpoint 'arena-dev.targetpractice.network:11820', got '%s'", cfg.ServerEndpoint)
	}
	// Check that AllowedIpsForRouting contains VPN subnet and service IP
	expectedIPs := []string{"10.0.0.0/24", "10.0.0.1/32"}
	if len(cfg.AllowedIpsForRouting) != len(expectedIPs) {
		t.Errorf("expected %d allowed IPs for routing, got %d: %v", len(expectedIPs), len(cfg.AllowedIpsForRouting), cfg.AllowedIpsForRouting)
	}
	for _, expectedIP := range expectedIPs {
		found := false
		for _, actualIP := range cfg.AllowedIpsForRouting {
			if actualIP == expectedIP {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected allowed IPs for routing to contain '%s', got %v", expectedIP, cfg.AllowedIpsForRouting)
		}
	}
	if !k8s.CreateNamespaceCalled || !k8s.ValidateSpecCalled || !k8s.CreateResourcesFromSpecCalled || !k8s.ApplyNetworkPolicyCalled {
		t.Error("expected all k8s methods to be called")
	}
	if !vpn.CreatePeerCalled {
		t.Error("expected vpn CreatePeer to be called")
	}
}

func TestArenaManager_CreateArena_K8sError(t *testing.T) {
	k8s := &mockK8sManager{CreateNamespaceErr: errors.New("fail")}
	vpn := &mockVPNManager{}
	mgr := NewArenaManager(k8s, vpn)
	_, err := mgr.CreateArena("pk", "10.0.0.2/32", "test-challenge", "spec")
	if err == nil || err.Error() != "failed to create namespace: fail" {
		t.Errorf("expected namespace error, got %v", err)
	}
}

func TestArenaManager_DestroyArena_Success(t *testing.T) {
	k8s := &mockK8sManager{}
	vpn := &mockVPNManager{}
	mgr := NewArenaManager(k8s, vpn)
	err := mgr.DestroyArena("pk")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !vpn.RemovePeerCalled || !k8s.DeleteNamespaceCalled {
		t.Error("expected RemovePeer and DeleteNamespace to be called")
	}
}

func TestArenaManager_DestroyArena_VPNError(t *testing.T) {
	k8s := &mockK8sManager{}
	vpn := &mockVPNManager{RemovePeerErr: errors.New("fail")}
	mgr := NewArenaManager(k8s, vpn)
	err := mgr.DestroyArena("pk")
	if err == nil || err.Error() != "failed to remove VPN peer: fail" {
		t.Errorf("expected vpn error, got %v", err)
	}
}

func TestArenaManager_GetArenaStatus_Success(t *testing.T) {
	k8s := &mockK8sManager{
		GetNamespaceStatusCalled: true,
		NamespaceStatus: &NamespaceStatus{
			Name:  "arena-cgs",
			Phase: "Active",
		},
	}
	vpn := &mockVPNManager{PeerStatus: &PeerStatus{PublicKey: "pk"}}
	mgr := NewArenaManager(k8s, vpn)
	status, err := mgr.GetArenaStatus("pk")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !vpn.GetPeerStatusCalled {
		t.Error("expected GetPeerStatus to be called")
	}
	if !k8s.GetNamespaceStatusCalled {
		t.Error("expected GetNamespaceStatus to be called")
	}
	if status.GetVpnStatus() == nil {
		t.Error("expected vpn_status in arena status")
	}
	if status.GetK8SStatus() == nil {
		t.Error("expected k8s_status in arena status")
	}
	// The namespace is computed from the public key hash
	expectedNamespace := "arena-5myqfjwllbtwlua7vuzeki7m" // pubKeyToNamespace("pk")
	if status.GetNamespace() != expectedNamespace {
		t.Errorf("expected namespace %s, got %v", expectedNamespace, status.GetNamespace())
	}
	if status.GetPublicKey() != "pk" {
		t.Errorf("expected public_key 'pk', got %v", status.GetPublicKey())
	}
}

func TestArenaManager_GetArenaStatus_VPNError(t *testing.T) {
	vpn := &mockVPNManager{GetPeerStatusErr: errors.New("fail")}
	mgr := NewArenaManager(&mockK8sManager{}, vpn)
	_, err := mgr.GetArenaStatus("pk")
	if err == nil || err.Error() != "failed to get VPN peer status: fail" {
		t.Errorf("expected error, got %v", err)
	}
}

func TestArenaManager_GetArenaStatus_K8sError(t *testing.T) {
	k8s := &mockK8sManager{GetNamespaceStatusErr: errors.New("fail")}
	vpn := &mockVPNManager{PeerStatus: &PeerStatus{PublicKey: "pk"}}
	mgr := NewArenaManager(k8s, vpn)
	_, err := mgr.GetArenaStatus("pk")
	if err == nil || err.Error() != "failed to get namespace status: fail" {
		t.Errorf("expected error, got %v", err)
	}
}
