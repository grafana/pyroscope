package service

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type NoopApplicationService struct{}

func (NoopApplicationService) CreateOrUpdate(context.Context, storage.Application) error {
	return nil
}

func (NoopApplicationService) List(context.Context, storage.Application) (apps []storage.Application, err error) {
	return apps, err
}

func (NoopApplicationService) Get(context.Context, string) (app storage.Application, err error) {
	return app, err
}

func (NoopApplicationService) Delete(context.Context, string) error {
	return nil
}
