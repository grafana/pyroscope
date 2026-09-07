package querybackend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/grafana/pyroscope/v2/pkg/objstore"
)

const (
	resultCacheRedisMaxValueSize = 16 * 1024
	resultCacheRedisKeyPrefix    = "pyroscope:result-cache:"
	resultCacheRedisRecordInline = byte(1)
	resultCacheRedisRecordObject = byte(2)
)

var (
	errResultCacheNotFound = errors.New("result cache entry not found")
	errResultCacheUnknown  = errors.New("result cache state unknown")
	errResultCacheCorrupt  = errors.New("corrupt result cache entry")
)

type ResultCacheStore interface {
	Get(context.Context, string) ([]byte, string, error)
	Put(context.Context, string, []byte, time.Duration) (string, error)
	Close() error
}

type redisResultCacheStore struct {
	bucket objstore.Bucket
	redis  redis.UniversalClient
}

func NewResultCacheStore(bucket objstore.Bucket, redisClient redis.UniversalClient) ResultCacheStore {
	return &redisResultCacheStore{bucket: bucket, redis: redisClient}
}

func (s *redisResultCacheStore) Get(ctx context.Context, key string) ([]byte, string, error) {
	record, err := s.redis.Get(ctx, resultCacheRedisKeyPrefix+key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, "redis", errResultCacheNotFound
		}
		if ctx.Err() != nil {
			return nil, "redis", ctx.Err()
		}
		data, objectErr := s.getObject(ctx, key)
		if objectErr == nil {
			return data, "object", nil
		}
		return nil, "redis", fmt.Errorf("%w: Redis: %v; object storage: %v", errResultCacheUnknown, err, objectErr)
	}
	if len(record) == 0 {
		return nil, "redis", errResultCacheCorrupt
	}
	switch record[0] {
	case resultCacheRedisRecordInline:
		return record[1:], "redis", nil
	case resultCacheRedisRecordObject:
		data, err := s.getObject(ctx, key)
		if ctx.Err() != nil {
			return nil, "object", ctx.Err()
		}
		if errors.Is(err, errResultCacheNotFound) {
			return nil, "object", errResultCacheNotFound
		}
		if err != nil {
			return nil, "object", fmt.Errorf("%w: %v", errResultCacheUnknown, err)
		}
		return data, "object", nil
	default:
		return nil, "redis", errResultCacheCorrupt
	}
}

func (s *redisResultCacheStore) Put(ctx context.Context, key string, data []byte, ttl time.Duration) (string, error) {
	if len(data) < resultCacheRedisMaxValueSize {
		record := append([]byte{resultCacheRedisRecordInline}, data...)
		return "redis", s.redis.Set(ctx, resultCacheRedisKeyPrefix+key, record, ttl).Err()
	}
	if err := s.bucket.Upload(ctx, key, bytes.NewReader(data)); err != nil {
		return "object", err
	}
	if err := s.redis.Set(ctx, resultCacheRedisKeyPrefix+key, []byte{resultCacheRedisRecordObject}, ttl).Err(); err != nil {
		return "redis", err
	}
	return "object", nil
}

func (s *redisResultCacheStore) getObject(ctx context.Context, key string) ([]byte, error) {
	r, err := s.bucket.Get(ctx, key)
	if err != nil {
		if s.bucket.IsObjNotFoundErr(err) {
			return nil, errResultCacheNotFound
		}
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func (s *redisResultCacheStore) Close() error {
	redisErr := s.redis.Close()
	bucketErr := s.bucket.Close()
	return errors.Join(redisErr, bucketErr)
}
