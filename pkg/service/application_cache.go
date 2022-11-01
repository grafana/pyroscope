package service

import (
	"context"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type ApplicationCacheService struct {
	appSvc ApplicationService
	cache  *cache
}

type ApplicationCacheServiceConfig struct {
	Size int
	TTL  time.Duration
}

func NewApplicationCacheService(config ApplicationCacheServiceConfig, appSvc ApplicationService) *ApplicationCacheService {
	if config.Size <= 0 {
		config.Size = 100
	}

	if config.TTL <= 0 {
		config.TTL = 5 * time.Minute
	}

	cache := newCache(config.Size, config.TTL)
	return &ApplicationCacheService{appSvc: appSvc, cache: cache}
}

func (svc ApplicationCacheService) List(ctx context.Context) (apps []storage.Application, err error) {
	return apps, nil
}

func (svc ApplicationCacheService) Get(ctx context.Context, name string) (storage.Application, error) {
	app := storage.Application{}
	return app, nil
}

func (svc ApplicationCacheService) CreateOrUpdate(ctx context.Context, application storage.Application) error {
	if err := model.ValidateAppName(application.Name); err != nil {
		return err
	}

	return nil
}

func (svc ApplicationCacheService) Delete(ctx context.Context, name string) error {
	if err := model.ValidateAppName(name); err != nil {
		return err
	}

	return nil
}
