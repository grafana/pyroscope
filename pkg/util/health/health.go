package health

import (
	"google.golang.org/grpc/health/grpc_health_v1"
)

type Server interface {
	SetServingStatus(string, grpc_health_v1.HealthCheckResponse_ServingStatus)
}

type noopServer struct{}

var NoOpServer = noopServer{}

func (noopServer) SetServingStatus(string, grpc_health_v1.HealthCheckResponse_ServingStatus) {}
