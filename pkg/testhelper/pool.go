package testhelper

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type FakePoolClient struct{}

func (f FakePoolClient) Close() error {
	return nil
}

func (f FakePoolClient) Check(ctx context.Context, in *grpc_health_v1.HealthCheckRequest, opts ...grpc.CallOption) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_SERVING,
	}, nil
}

func (f FakePoolClient) Watch(ctx context.Context, in *grpc_health_v1.HealthCheckRequest, opts ...grpc.CallOption) (grpc_health_v1.Health_WatchClient, error) {
	return nil, errors.New("not implemented")
}
