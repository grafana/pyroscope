package service

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type ApplicationService struct{}

func (ApplicationService) List(ctx context.Context) ([]storage.Application, error) {
	//TODO implement me
	panic("implement me")
}

func (ApplicationService) Get(ctx context.Context, name string) (storage.Application, error) {
	//TODO implement me
	panic("implement me")
}

func (ApplicationService) CreateOrUpdate(ctx context.Context, application storage.Application) error {
	//TODO implement me
	panic("implement me")
}

func (ApplicationService) Delete(ctx context.Context, name string) error {
	//TODO implement me
	panic("implement me")
}
