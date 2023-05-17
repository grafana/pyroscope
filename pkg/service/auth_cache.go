package service

import (
	"context"
	"time"

	lru "github.com/hashicorp/golang-lru"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

type CachingAuthService struct {
	AuthService
	cache *cache
}

type CachingAuthServiceConfig struct {
	Size int
	TTL  time.Duration
}

func NewCachingAuthService(authService AuthService, c CachingAuthServiceConfig) CachingAuthService {
	cas := CachingAuthService{AuthService: authService}
	if c.Size > 0 && c.TTL > 0 {
		cas.cache = newCache(c.Size, c.TTL)
	}
	return cas
}

func (svc CachingAuthService) APIKeyFromToken(ctx context.Context, t string) (model.APIKey, error) {
	if svc.cache != nil {
		return svc.cachedAPIKeyFromToken(ctx, t)
	}
	return svc.AuthService.APIKeyFromToken(ctx, t)
}

func (svc CachingAuthService) cachedAPIKeyFromToken(ctx context.Context, t string) (model.APIKey, error) {
	if v, ok := svc.cache.get(t); ok {
		switch x := v.(type) {
		case error:
			return model.APIKey{}, x
		case model.APIKey:
			return x, nil
		}
	}
	k, err := svc.AuthService.APIKeyFromToken(ctx, t)
	if err != nil {
		svc.cache.put(t, err)
		return k, err
	}
	svc.cache.put(t, k)
	return k, err
}

func (svc CachingAuthService) PutAPIKey(t string, k model.APIKey) {
	if svc.cache != nil {
		svc.cache.put(t, k)
	}
}

func (svc CachingAuthService) DeleteAPIKey(t string) {
	if svc.cache != nil {
		svc.cache.c.Remove(t)
	}
}

// TODO(kolesnikovae): Move to a separate package.

type cache struct {
	ttl time.Duration
	c   *lru.Cache
}

type cachedItem struct {
	value   interface{}
	created time.Time
}

func newCache(size int, ttl time.Duration) *cache {
	c := cache{ttl: ttl}
	c.c, _ = lru.New(size)
	return &c
}

func (c *cache) put(k string, v interface{}) {
	c.c.Add(k, cachedItem{
		created: time.Now(),
		value:   v,
	})
}

func (c *cache) get(k string) (interface{}, bool) {
	x, found := c.c.Get(k)
	if !found {
		return nil, false
	}
	if v, ok := x.(cachedItem); ok && time.Since(v.created) < c.ttl {
		return v.value, ok
	}
	c.c.Remove(k) // Expired.
	return nil, false
}
