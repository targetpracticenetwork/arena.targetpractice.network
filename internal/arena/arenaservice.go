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

	cfg, err := s.Manager.CreateArena(req.PublicKey, req.VpnIp, req.ChallengeName, req.ChallengeSpec)
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
	log.Printf("🗑️ DestroyArena request received - PublicKey: %s", req.PublicKey)

	err := s.Manager.DestroyArena(req.PublicKey)
	if err != nil {
		log.Printf("❌ DestroyArena failed - PublicKey: %s, Error: %v", req.PublicKey, err)
		return &DestroyArenaResponse{Success: false, Error: err.Error()}, nil
	}

	log.Printf("✅ DestroyArena successful - PublicKey: %s", req.PublicKey)
	return &DestroyArenaResponse{Success: true}, nil
}

func (s *ArenaServiceServerImpl) GetArenaStatus(ctx context.Context, req *GetArenaStatusRequest) (*GetArenaStatusResponse, error) {
	log.Printf("📊 GetArenaStatus request received - PublicKey: %s", req.PublicKey)

	status, err := s.Manager.GetArenaStatus(req.PublicKey)
	if err != nil {
		log.Printf("❌ GetArenaStatus failed - PublicKey: %s, Error: %v", req.PublicKey, err)
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
	log.Printf("📊 GetArenaStatus successful - PublicKey: %s, Namespace: %s, Healthy: %s, Pods: %d, Services: %d",
		req.PublicKey, status.Namespace, healthy, status.GetK8SStatus().GetPodCount(), status.GetK8SStatus().GetServiceCount())

	// Optionally, include health information in the error field if unhealthy
	if !status.GetHealthy() && len(status.GetErrors()) > 0 {
		response.Error = "Arena unhealthy: " + status.GetErrors()[0]
	}

	return response, nil
}
