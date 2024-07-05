package querybackend

import (
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
)

type merger struct{}

func (m *merger) merge(resp *querybackendv1.InvokeResponse, err error) error {
	if err != nil {
		return err
	}
	return nil
}

func (m *merger) response() (*querybackendv1.InvokeResponse, error) {
	return new(querybackendv1.InvokeResponse), nil
}
