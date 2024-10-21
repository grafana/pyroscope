package health

import (
	"context"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type Service interface {
	SetServing()
	SetNotServing()
}

type noopService struct{}

var NoOpService = noopService{}

func (noopService) SetServing() {}

func (noopService) SetNotServing() {}

type GRPCHealthService struct {
	logger log.Logger
	name   string
	server *health.Server
}

func NewGRPCHealthService(server *health.Server, logger log.Logger, name string) *GRPCHealthService {
	return &GRPCHealthService{
		logger: logger,
		name:   name,
		server: server,
	}
}

func (s *GRPCHealthService) setStatus(x grpc_health_v1.HealthCheckResponse_ServingStatus) {
	level.Info(s.logger).Log("msg", "setting health status", "status", x)
	s.server.SetServingStatus(s.name, x)
}

func (s *GRPCHealthService) SetServing() {
	s.setStatus(grpc_health_v1.HealthCheckResponse_SERVING)
}

func (s *GRPCHealthService) SetNotServing() {
	s.setStatus(grpc_health_v1.HealthCheckResponse_NOT_SERVING)
}

var NoOpClient = noOpClient{}

type noOpClient struct{}

func (noOpClient) Check(context.Context, *grpc_health_v1.HealthCheckRequest, ...grpc.CallOption) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

func (noOpClient) Watch(context.Context, *grpc_health_v1.HealthCheckRequest, ...grpc.CallOption) (grpc_health_v1.Health_WatchClient, error) {
	return nil, errors.New("not implemented")
}
