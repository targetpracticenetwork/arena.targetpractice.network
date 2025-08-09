package arena

import (
	"context"
	"log"
)

// ArenaServiceServerImpl implements the gRPC ArenaServiceServer interface.
type ArenaServiceServerImpl struct {
	UnimplementedArenaServiceServer
	Manager ArenaManager
}

func NewArenaServiceServer(manager ArenaManager) *ArenaServiceServerImpl {
	return &ArenaServiceServerImpl{Manager: manager}
}

func (s *ArenaServiceServerImpl) CreateArena(ctx context.Context, req *CreateArenaRequest) (*CreateArenaResponse, error) {
	log.Printf("🎯 CreateArena request received - PublicKey: %s, VPN IP: %s, Challenge: %s",
		req.PublicKey, req.VpnIp, req.ChallengeName)

	cfg, err := s.Manager.CreateArena(ctx, req.PublicKey, req.VpnIp, req.ChallengeName, req.ChallengeSpec)
	if err != nil {
		log.Printf("❌ CreateArena failed - PublicKey: %s, Error: %v", req.PublicKey, err)
		return &CreateArenaResponse{Error: err.Error()}, nil
	}

	log.Printf("✅ CreateArena successful - PublicKey: %s, Namespace: %s, Endpoints: %d",
		req.PublicKey, cfg.Namespace, len(cfg.Endpoints))
	return &CreateArenaResponse{
		Config: cfg,
	}, nil
}

func (s *ArenaServiceServerImpl) DestroyArena(ctx context.Context, req *DestroyArenaRequest) (*DestroyArenaResponse, error) {
	log.Printf("🗑️ DestroyArena request received - VPN IP: %s", req.VpnIp)

	err := s.Manager.DestroyArena(ctx, req.VpnIp)
	if err != nil {
		log.Printf("❌ DestroyArena failed - VPN IP: %s, Error: %v", req.VpnIp, err)
		return &DestroyArenaResponse{Success: false, Error: err.Error()}, nil
	}

	log.Printf("✅ DestroyArena successful - VPN IP: %s", req.VpnIp)
	return &DestroyArenaResponse{Success: true}, nil
}

func (s *ArenaServiceServerImpl) GetArenaStatus(ctx context.Context, req *GetArenaStatusRequest) (*GetArenaStatusResponse, error) {
	log.Printf("📊 GetArenaStatus request received - VPN IP: %s", req.VpnIp)

	status, err := s.Manager.GetArenaStatus(ctx, req.VpnIp)
	if err != nil {
		log.Printf("❌ GetArenaStatus failed - VPN IP: %s, Error: %v", req.VpnIp, err)
		return &GetArenaStatusResponse{Error: err.Error()}, nil
	}

	response := &GetArenaStatusResponse{
		Status: status,
	}

	// Log status summary
	healthy := "✅"
	if !status.GetHealthy() {
		healthy = "❌"
	}
	log.Printf("📊 GetArenaStatus successful - VPN IP: %s, Namespace: %s, Healthy: %s, Pods: %d, Services: %d",
		req.VpnIp, status.Namespace, healthy, status.GetK8SStatus().GetPodCount(), status.GetK8SStatus().GetServiceCount())

	// Optionally, include health information in the error field if unhealthy
	if !status.GetHealthy() && len(status.GetErrors()) > 0 {
		response.Error = "Arena unhealthy: " + status.GetErrors()[0]
	}

	return response, nil
}
