package service

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

type ApplicationService struct{}

func (ApplicationService) List(ctx context.Context) ([]model.Application, error) {
	//TODO implement me
	panic("implement me")
}

func (ApplicationService) Get(ctx context.Context, name string) (model.Application, error) {
	//TODO implement me
	panic("implement me")
}

func (ApplicationService) CreateOrUpdate(ctx context.Context, application model.Application) error {
	//TODO implement me
	panic("implement me")
}

func (ApplicationService) Delete(ctx context.Context, name string) error {
	//TODO implement me
	panic("implement me")
}
