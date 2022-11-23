package service

import (
	"context"
	"reflect"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/model/appmetadata"
)

type ApplicationMetadataWriter interface {
	CreateOrUpdate(ctx context.Context, application appmetadata.ApplicationMetadata) error
}

type ApplicationMetadataCacheService struct {
	appSvc ApplicationMetadataWriter
	cache  *cache
}

type ApplicationMetadataCacheServiceConfig struct {
	Size int
	TTL  time.Duration
}

func NewApplicationMetadataCacheService(config ApplicationMetadataCacheServiceConfig, appSvc ApplicationMetadataWriter) *ApplicationMetadataCacheService {
	if config.Size <= 0 {
		config.Size = 1000
	}

	if config.TTL <= 0 {
		config.TTL = 5 * time.Minute
	}

	cache := newCache(config.Size, config.TTL)
	return &ApplicationMetadataCacheService{appSvc: appSvc, cache: cache}
}

// CreateOrUpdate delegates to the underlying service in the following cases:
// * item is not in the cache
// * data is different from what's in the cache
// Otherwise it does nothing
func (svc *ApplicationMetadataCacheService) CreateOrUpdate(ctx context.Context, application appmetadata.ApplicationMetadata) error {
	if cachedApp, ok := svc.cache.get(application.FQName); ok {
		if !svc.isTheSame(application, cachedApp.(appmetadata.ApplicationMetadata)) {
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
func (svc *ApplicationMetadataCacheService) writeToBoth(ctx context.Context, application appmetadata.ApplicationMetadata) error {
	if err := svc.appSvc.CreateOrUpdate(ctx, application); err != nil {
		return err
	}
	svc.cache.put(application.FQName, application)
	return nil
}

// isTheSame check if 2 applications have the same data
// TODO(eh-am): update to a more robust comparison function
// See https://pkg.go.dev/reflect#DeepEqual for its drawbacks
func (*ApplicationMetadataCacheService) isTheSame(app1 appmetadata.ApplicationMetadata, app2 appmetadata.ApplicationMetadata) bool {
	return reflect.DeepEqual(app1, app2)
}
