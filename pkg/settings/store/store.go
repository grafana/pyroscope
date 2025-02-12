package store

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"sync"

	"github.com/go-kit/log"
	"github.com/thanos-io/objstore"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Key struct {
	TenantID string
}

type GenericStore[T any, PT interface {
	proto.Message
	*T
}] struct {
	logger log.Logger
	bucket objstore.Bucket
	path   string

	cacheLock sync.RWMutex
	cache     PT
}

type StoreType interface {
	proto.Message
}

func New[T any, PT interface {
	StoreType
	*T
}](
	logger log.Logger, bucket objstore.Bucket, key Key, path string,
) *GenericStore[T, PT] {
	return &GenericStore[T, PT]{
		logger: logger,
		bucket: bucket,
		path:   filepath.Join(key.TenantID, path) + ".json",
	}
}

func (s *GenericStore[T, PT]) Get(ctx context.Context) (PT, error) {
	// serve from cache if available
	s.cacheLock.RLock()
	if s.cache != nil {
		s.cacheLock.RUnlock()
		return s.cache, nil
	}
	s.cacheLock.RUnlock()

	// get write lock and fetch from bucket
	s.cacheLock.Lock()
	defer s.cacheLock.Unlock()

	// check again if cache is available in the meantime
	if s.cache != nil {
		return s.cache, nil
	}

	// load from bucket
	if err := s.unsafeLoadCache(ctx); err != nil {
		return nil, err
	}

	return s.cache, nil
}

// If the update callback returns this, it will not update, but it also won't return an error
var ErrUpdateAbort = errors.New("abort update")

// Update will under write lock, call a callback that updates the store. If there is an error returned, the update will be cancelled.
func (s *GenericStore[T, PT]) Update(
	ctx context.Context,
	updateF func(v PT) error,
) error {
	// get write lock and fetch from bucket
	s.cacheLock.Lock()
	defer s.cacheLock.Unlock()

	// ensure we have the latest data
	data, err := s.getFromBucket(ctx)
	if err != nil {
		return err
	}

	// call callback
	if err := updateF(data); err == ErrUpdateAbort {
		return nil
	} else if err != nil {
		return err
	}

	// save the changes
	return s.unsafeFlush(ctx, data)
}

func (s *GenericStore[T, PT]) getFromBucket(ctx context.Context) (PT, error) {
	// fetch from bucket
	r, err := s.bucket.Get(ctx, s.path)
	if s.bucket.IsObjNotFoundErr(err) {
		return new(T), nil
	}
	if err != nil {
		return nil, err
	}

	bodyJson, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// unmarshal the data
	var data PT = new(T)
	if err := protojson.Unmarshal(bodyJson, data); err != nil {
		return nil, err
	}

	return data, nil
}

// unsafeLoad reads from bucket into the cache, only call with write lock held
func (s *GenericStore[T, PT]) unsafeLoadCache(ctx context.Context) error {
	// fetch from bucket
	data, err := s.getFromBucket(ctx)
	if err != nil {
		return err
	}

	s.cache = data
	return nil
}

// unsafeFlush writes from arguments into the bucket and then reset cache. Only call with write lock held
func (s *GenericStore[T, PT]) unsafeFlush(ctx context.Context, data PT) error {
	// increment generation
	// TODO:	data.SetGeneration(data.GetGeneration() + 1)

	// marshal the data
	dataJson, err := protojson.Marshal(data)
	if err != nil {
		return err
	}

	// reset cache
	s.cache = nil

	// write to bucket
	return s.bucket.Upload(ctx, s.path, bytes.NewReader(dataJson))
}
