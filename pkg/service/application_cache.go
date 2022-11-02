package service

import (
	"context"
	"fmt"
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

// CreateOrUpdate delegates to the underlying service
// Only when data is different from what's in the cache/is not in the cache
// Otherwise it does nothing
func (svc *ApplicationCacheService) CreateOrUpdate(ctx context.Context, application storage.Application) error {
	if _, ok := svc.cache.get(application.Name); ok {
		fmt.Println("data is in cache")
		// Is in cache
		// TODO: Is it different from what's in the cache?
		// If so
		// Write to cache
		// Write to underlying store
		// Otherwise, don't do anything
		//return svc.writeToBoth(ctx, application)
		return nil
	}
	fmt.Println("data is not in cache")

	// Not in cache
	// Could be due to TTL
	// Or could it be that's a new app
	return svc.writeToBoth(ctx, application)
}

func (svc *ApplicationCacheService) writeToBoth(ctx context.Context, application storage.Application) error {
	if err := svc.appSvc.CreateOrUpdate(ctx, application); err != nil {
		return err
	}
	fmt.Println("writing to cache")
	svc.cache.put(application.Name, application)
	g, ok := svc.cache.get(application.Name)
	fmt.Println("wrote to cache application", application.Name)
	fmt.Println("wrote to cache", g)
	fmt.Println("wrote to cache ok", ok)
	return nil
}

//func (svc ApplicationCacheService) Delete(ctx context.Context, name string) error {
//	if err := model.ValidateAppName(name); err != nil {
//		return err
//	}
//
//	return nil
//}
