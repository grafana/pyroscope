package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/go-kit/log"
	"github.com/thanos-io/objstore"
	"google.golang.org/protobuf/proto"
)

type Key struct {
	TenantID string
}

type StoreHelper[T any] interface {
	ID(T) string
	GetGeneration(T) int64
	SetGeneration(T, int64)
	FromStore(json.RawMessage) (T, error)
	ToStore(T) (json.RawMessage, error)
	TypePath() string
}

type Collection[T any] struct {
	Generation int64
	Elements   []T
}

type StoreType interface {
	proto.Message
}

type GenericStore[T StoreType, H StoreHelper[T]] struct {
	logger log.Logger
	bucket objstore.Bucket
	helper H
	path   string

	cacheLock sync.RWMutex
	cache     *Collection[T]
}

func New[T StoreType, H StoreHelper[T]](
	logger log.Logger, bucket objstore.Bucket, key Key, helper H,
) *GenericStore[T, H] {
	return &GenericStore[T, H]{
		logger: logger,
		bucket: bucket,
		helper: helper,
		path:   filepath.Join(key.TenantID, helper.TypePath()) + ".json",
	}
}

func (s *GenericStore[T, H]) Get(ctx context.Context) (*Collection[T], error) {
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

// Update will under write lock, call a callback that updates the store. If there is an error returned, the update will be cancelled.
func (s *GenericStore[T, H]) Update(
	ctx context.Context,
	updateF func(col *Collection[T]) error,
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
	if err := updateF(data); err != nil {
		return err
	}

	// save the changes
	return s.unsafeFlush(ctx, data)
}

type storeStruct struct {
	Generation     string            `json:"generation"`
	Elements       []json.RawMessage `json:"elements,omitempty"`
	ElementsCompat []json.RawMessage `json:"rules,omitempty"`
}

func (s *GenericStore[T, H]) getFromBucket(ctx context.Context) (*Collection[T], error) {
	// fetch from bucket
	r, err := s.bucket.Get(ctx, s.path)
	if s.bucket.IsObjNotFoundErr(err) {
		return &Collection[T]{
			Elements: make([]T, 0),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	var storeStruct storeStruct
	if err := json.NewDecoder(r).Decode(&storeStruct); err != nil {
		return nil, err
	}

	// handle compatibility with old model
	if len(storeStruct.Elements) == 0 {
		storeStruct.Elements = storeStruct.ElementsCompat
	}

	var (
		result = make([]T, len(storeStruct.Elements))
	)
	for idx, element := range storeStruct.Elements {
		result[idx], err = s.helper.FromStore(element)
		if err != nil {
			return nil, err
		}
	}

	generation, err := strconv.ParseInt(storeStruct.Generation, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid generation: %s", storeStruct.Generation)
	}

	return &Collection[T]{
		Generation: generation,
		Elements:   result,
	}, nil
}

// unsafeLoad reads from bucket into the cache, only call with write lock held
func (s *GenericStore[T, H]) unsafeLoadCache(ctx context.Context) error {
	// fetch from bucket
	data, err := s.getFromBucket(ctx)
	if err != nil {
		return err
	}

	s.cache = data
	return nil
}

// unsafeFlush writes from arguments into the bucket and then reset cache. Only call with write lock held
func (s *GenericStore[T, H]) unsafeFlush(ctx context.Context, coll *Collection[T]) error {
	var (
		data = storeStruct{
			Elements:   make([]json.RawMessage, len(coll.Elements)),
			Generation: strconv.FormatInt(coll.Generation+1, 10),
		}
		err error
	)
	for idx, element := range coll.Elements {
		data.Elements[idx], err = s.helper.ToStore(element)
		if err != nil {
			return err
		}
	}

	dataJson, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// reset cache
	s.cache = nil

	// write to bucket
	return s.bucket.Upload(ctx, s.path, bytes.NewReader(dataJson))
}
