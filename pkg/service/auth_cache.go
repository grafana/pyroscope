package service

import (
	"context"
	"errors"
	"time"

	"github.com/hashicorp/golang-lru"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

type CachingAuthService struct {
	AuthService
	p, n cache
}

type CachingAuthServiceConfig struct {
	NegativeSize int
	PositiveSize int
	NegativeTTL  time.Duration
	PositiveTTL  time.Duration
}

func NewCachingAuthService(authService AuthService, c CachingAuthServiceConfig) CachingAuthService {
	return CachingAuthService{
		AuthService: authService,

		p: newCache(c.PositiveSize, c.PositiveTTL),
		n: newCache(c.NegativeSize, c.NegativeTTL),
	}
}

func (svc CachingAuthService) PutAPIKey(t string, k model.APIKeyToken) { svc.p.put(t, k) }

func (svc CachingAuthService) InvalidateAPIKey(t string) { svc.n.put(t, nil) }

func (svc CachingAuthService) APIKeyTokenFromJWTToken(ctx context.Context, t string) (model.APIKeyToken, error) {
	if _, ok := svc.n.get(t); ok {
		return model.APIKeyToken{}, model.ErrAPIKeyNotFound
	}
	if v, ok := svc.p.get(t); ok {
		return v.(model.APIKeyToken), nil
	}
	v, err := svc.AuthService.APIKeyTokenFromJWTToken(ctx, t)
	if errors.Is(err, model.ErrAPIKeyNotFound) {
		svc.n.put(t, nil)
	}
	if err != nil {
		return model.APIKeyToken{}, err
	}
	svc.p.put(t, v)
	return v, nil
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

func newCache(size int, ttl time.Duration) cache {
	c := cache{ttl: ttl}
	c.c, _ = lru.New(size)
	return c
}

func (c cache) put(k string, v interface{}) {
	c.c.Add(k, cachedItem{
		created: time.Now(),
		value:   v,
	})
}

func (c cache) get(k string) (interface{}, bool) {
	x, found := c.c.Get(k)
	if found {
		return nil, false
	}
	if v, ok := x.(cachedItem); ok && time.Since(v.created) < c.ttl {
		return v.value, ok
	}
	c.c.Remove(k) // Expired.
	return nil, false
}
