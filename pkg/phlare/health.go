package phlare

import (
	"context"

	grpchealth "github.com/bufbuild/connect-grpchealth-go"
	"github.com/gorilla/mux"
	"github.com/grafana/dskit/grpcutil"
)

type checker struct {
	checks []grpcutil.Check
}

func RegisterHealthServer(mux *mux.Router, checks ...grpcutil.Check) {
	prefix, handler := grpchealth.NewHandler(&checker{checks: checks})
	mux.NewRoute().PathPrefix(prefix).Handler(handler)
}

func (c *checker) Check(ctx context.Context, req *grpchealth.CheckRequest) (*grpchealth.CheckResponse, error) {
	for _, check := range c.checks {
		if !check(ctx) {
			return &grpchealth.CheckResponse{Status: grpchealth.StatusNotServing}, nil
		}
	}
	return &grpchealth.CheckResponse{Status: grpchealth.StatusServing}, nil
}
