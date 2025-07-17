package arena

import (
	context "context"
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
	cfg, err := s.Manager.CreateArena(req.PublicKey, req.VpnIp, req.ChallengeName, req.ChallengeSpec)
	if err != nil {
		return &CreateArenaResponse{Error: err.Error()}, nil
	}
	return &CreateArenaResponse{
		Config: cfg,
	}, nil
}

func (s *ArenaServiceServerImpl) DestroyArena(ctx context.Context, req *DestroyArenaRequest) (*DestroyArenaResponse, error) {
	err := s.Manager.DestroyArena(req.PublicKey)
	if err != nil {
		return &DestroyArenaResponse{Success: false, Error: err.Error()}, nil
	}
	return &DestroyArenaResponse{Success: true}, nil
}

func (s *ArenaServiceServerImpl) GetArenaStatus(ctx context.Context, req *GetArenaStatusRequest) (*GetArenaStatusResponse, error) {
	status, err := s.Manager.GetArenaStatus(req.PublicKey)
	if err != nil {
		return &GetArenaStatusResponse{Error: err.Error()}, nil
	}

	response := &GetArenaStatusResponse{
		Status: status,
	}

	// Optionally, include health information in the error field if unhealthy
	if !status.GetHealthy() && len(status.GetErrors()) > 0 {
		response.Error = "Arena unhealthy: " + status.GetErrors()[0]
	}

	return response, nil
}
