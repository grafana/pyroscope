package health

import (
	"context"

	"github.com/grafana/dskit/services"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type Service interface {
	SetServingStatus(string, grpc_health_v1.HealthCheckResponse_ServingStatus)
}

type noopService struct{}

var NoOpService = noopService{}

func (noopService) SetServingStatus(string, grpc_health_v1.HealthCheckResponse_ServingStatus) {}

func NewGRPCHealthService() *GRPCHealthService {
	s := health.NewServer()
	return &GRPCHealthService{
		Server: s,
		Service: services.NewIdleService(nil, func(error) error {
			s.Shutdown()
			return nil
		}),
	}
}

type GRPCHealthService struct {
	services.Service
	*health.Server
}

var NoOpClient = noOpClient{}

type noOpClient struct{}

func (noOpClient) Check(context.Context, *grpc_health_v1.HealthCheckRequest, ...grpc.CallOption) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

func (noOpClient) Watch(context.Context, *grpc_health_v1.HealthCheckRequest, ...grpc.CallOption) (grpc_health_v1.Health_WatchClient, error) {
	return nil, errors.New("not implemented")
}
