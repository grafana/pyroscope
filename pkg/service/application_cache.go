package service

import (
	"context"
	"reflect"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type ApplicationWriter interface {
	CreateOrUpdate(ctx context.Context, application storage.Application) error
}

type ApplicationCacheService struct {
	appSvc ApplicationWriter
	cache  *cache
}

type ApplicationCacheServiceConfig struct {
	Size int
	TTL  time.Duration
}

func NewApplicationCacheService(config ApplicationCacheServiceConfig, appSvc ApplicationWriter) *ApplicationCacheService {
	if config.Size <= 0 {
		config.Size = 100
	}

	if config.TTL <= 0 {
		config.TTL = 5 * time.Minute
	}

	cache := newCache(config.Size, config.TTL)
	return &ApplicationCacheService{appSvc: appSvc, cache: cache}
}

// CreateOrUpdate delegates to the underlying service in the following cases:
// * item is not in the cache
// * data is different from what's in the cache
// Otherwise it does nothing
func (svc *ApplicationCacheService) CreateOrUpdate(ctx context.Context, application storage.Application) error {
	if cachedApp, ok := svc.cache.get(application.Name); ok {
		if !svc.isTheSame(application, cachedApp.(storage.Application)) {
			return svc.writeToBoth(ctx, application)
		}
		return nil
	}

	// Not in cache
	// Could be due to TTL
	// Or could it be that's a new app
	return svc.writeToBoth(ctx, application)
}

// writeToBoth writes to both the cache and the underlying service
func (svc *ApplicationCacheService) writeToBoth(ctx context.Context, application storage.Application) error {
	if err := svc.appSvc.CreateOrUpdate(ctx, application); err != nil {
		return err
	}
	svc.cache.put(application.Name, application)
	return nil
}

// isTheSame check if 2 applications have the same data
// TODO(eh-am): update to a more robust comparison function
// See https://pkg.go.dev/reflect#DeepEqual for its drawbacks
func (*ApplicationCacheService) isTheSame(app1 storage.Application, app2 storage.Application) bool {
	return reflect.DeepEqual(app1, app2)
}
