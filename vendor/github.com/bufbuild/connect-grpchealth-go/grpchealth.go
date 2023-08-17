// Copyright 2022 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package grpchealth enables any net/http server, including those built with
// Connect, to respond to gRPC-style health checks. This lets load balancers,
// container orchestrators, and other infrastructure systems respond to changes
// in your HTTP server's health.
//
// The exposed health-checking API is wire compatible with Google's gRPC
// implementations, so it works with grpcurl, grpc-health-probe, and Kubernetes
// gRPC liveness probes.
//
// The core Connect package is github.com/bufbuild/connect-go. Documentation is
// available at https://connect.build.
package grpchealth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/bufbuild/connect-go"
	healthv1 "github.com/bufbuild/connect-grpchealth-go/internal/gen/go/connectext/grpc/health/v1"
)

// Status describes the health of a service.
type Status uint8

const (
	// StatusUnknown indicates that the service's health state is indeterminate.
	StatusUnknown Status = 0

	// StatusServing indicates that the service is ready to accept requests.
	StatusServing Status = 1

	// StatusNotServing indicates that the process is healthy but the service is
	// not accepting requests. For example, StatusNotServing is often appropriate
	// when your primary database is down or unreachable.
	StatusNotServing Status = 2
)

// NewHandler wraps the supplied Checker to build an HTTP handler for gRPC's
// health-checking API. It returns the path on which to mount the handler and
// the HTTP handler itself.
//
// Note that the returned handler only supports the unary Check method, not the
// streaming Watch. As suggested in gRPC's health schema, it returns
// connect.CodeUnimplemented for the Watch method.
//
// For more details on gRPC's health checking protocol, see
// https://github.com/grpc/grpc/blob/master/doc/health-checking.md and
// https://github.com/grpc/grpc/blob/master/src/proto/grpc/health/v1/health.proto.
func NewHandler(checker Checker, options ...connect.HandlerOption) (string, http.Handler) {
	const serviceName = "/grpc.health.v1.Health/"
	mux := http.NewServeMux()
	check := connect.NewUnaryHandler(
		serviceName+"Check",
		func(
			ctx context.Context,
			req *connect.Request[healthv1.HealthCheckRequest],
		) (*connect.Response[healthv1.HealthCheckResponse], error) {
			var checkRequest CheckRequest
			if req.Msg != nil {
				checkRequest.Service = req.Msg.Service
			}
			checkResponse, err := checker.Check(ctx, &checkRequest)
			if err != nil {
				return nil, err
			}
			return connect.NewResponse(&healthv1.HealthCheckResponse{
				Status: healthv1.HealthCheckResponse_ServingStatus(checkResponse.Status),
			}), nil
		},
		options...,
	)
	mux.Handle(serviceName+"Check", check)
	watch := connect.NewServerStreamHandler(
		serviceName+"Watch",
		func(
			_ context.Context,
			_ *connect.Request[healthv1.HealthCheckRequest],
			_ *connect.ServerStream[healthv1.HealthCheckResponse],
		) error {
			return connect.NewError(
				connect.CodeUnimplemented,
				errors.New("connect doesn't support watching health state"),
			)
		},
		options...,
	)
	mux.Handle(serviceName+"Watch", watch)
	return serviceName, mux
}

// CheckRequest is a request for the health of a service. When using protobuf,
// Service will be a fully-qualified service name (for example,
// "acme.ping.v1.PingService"). If the Service is an empty string, the caller
// is asking for the health status of whole process.
type CheckRequest struct {
	Service string
}

// CheckResponse reports the health of a service (or of the whole process). The
// only valid Status values are StatusUnknown, StatusServing, and
// StatusNotServing. When asked to report on the status of an unknown service,
// Checkers should return a connect.CodeNotFound error.
//
// Often, systems monitoring health respond to errors by restarting the
// process. They often respond to StatusNotServing by removing the process from
// a load balancer pool.
type CheckResponse struct {
	Status Status
}

// A Checker reports the health of a service. It must be safe to call
// concurrently.
type Checker interface {
	Check(context.Context, *CheckRequest) (*CheckResponse, error)
}

// StaticChecker is a simple Checker implementation. It always returns
// StatusServing for the process, and it returns a static value for each
// service.
//
// If you have a dynamic list of services, want to ping a database as part of
// your health check, or otherwise need something more specialized, you should
// write a custom Checker implementation.
type StaticChecker struct {
	mu       sync.RWMutex
	statuses map[string]Status
}

// NewStaticChecker constructs a StaticChecker. By default, each of the
// supplied services has StatusServing.
//
// The supplied strings should be fully-qualified protobuf service names (for
// example, "acme.user.v1.UserService"). Generated Connect service files
// have this declared as a constant.
func NewStaticChecker(services ...string) *StaticChecker {
	statuses := make(map[string]Status, len(services))
	for _, service := range services {
		statuses[service] = StatusServing
	}
	return &StaticChecker{statuses: statuses}
}

// SetStatus sets the health status of a service, registering a new service if
// necessary. It's safe to call SetStatus and Check concurrently.
func (c *StaticChecker) SetStatus(service string, status Status) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.statuses[service] = status
}

// Check implements Checker. It's safe to call concurrently with SetStatus.
func (c *StaticChecker) Check(ctx context.Context, req *CheckRequest) (*CheckResponse, error) {
	if req.Service == "" {
		return &CheckResponse{Status: StatusServing}, nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if status, registered := c.statuses[req.Service]; registered {
		return &CheckResponse{Status: status}, nil
	}
	return nil, connect.NewError(
		connect.CodeNotFound,
		fmt.Errorf("unknown service %s", req.Service),
	)
}
