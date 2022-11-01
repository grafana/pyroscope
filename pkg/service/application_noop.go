package service

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type NoopApplicationService struct{}

func (NoopApplicationService) CreateOrUpdate(_ context.Context, _ storage.Application) error {
	return nil
}

func (NoopApplicationService) List(_ context.Context, _ storage.Application) (apps []storage.Application, err error) {
	return apps, err
}

func (NoopApplicationService) Get(ctx context.Context, name string) (app storage.Application, err error) {
	return app, err
}

func (NoopApplicationService) Delete(ctx context.Context, name string) error {
	return nil
}
