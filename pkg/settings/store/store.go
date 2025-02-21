package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/thanos-io/objstore"
)

type Key struct {
	TenantID string
}

var ErrElementNotFound = errors.New("element not found")

type ErrConflictGeneration struct {
	ObservedGeneration int64
	StoreGeneration    int64
}

func (e ErrConflictGeneration) Error() string {
	return fmt.Sprintf("conflicting update, please try again: observed_generation=%d, store_generation=%d", e.ObservedGeneration, e.StoreGeneration)
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

type GenericStore[T any, H StoreHelper[T]] struct {
	logger log.Logger
	bucket objstore.Bucket
	helper H
	path   string

	cacheLock sync.RWMutex
	cache     *Collection[T]
}

func New[T any, H StoreHelper[T]](
	logger log.Logger, bucket objstore.Bucket, key Key, helper H,
) *GenericStore[T, H] {
	return &GenericStore[T, H]{
		logger: logger,
		bucket: bucket,
		helper: helper,
		path:   filepath.Join(key.TenantID, helper.TypePath()) + ".json",
	}
}

// ReadTxn is a transaction that runs under the read lock of the cache. The Collection should not be mutated at all.
type ReadTxn[T any] func(context.Context, *Collection[T]) error

func (s *GenericStore[T, H]) Read(ctx context.Context, txn ReadTxn[T]) error {
	// serve from cache if available
	s.cacheLock.RLock()
	if s.cache != nil {
		defer s.cacheLock.RUnlock()
		return txn(ctx, s.cache)
	}
	s.cacheLock.RUnlock()

	// get write lock and fetch from bucket
	s.cacheLock.Lock()
	defer s.cacheLock.Unlock()

	// check again if cache is available in the meantime
	if s.cache != nil {
		return txn(ctx, s.cache)
	}

	// load from bucket
	if err := s.unsafeLoadCache(ctx); err != nil {
		return err
	}

	return txn(ctx, s.cache)
}

func (s *GenericStore[T, H]) Get(ctx context.Context) (*Collection[T], error) {
	var result *Collection[T]
	err := s.Read(ctx, func(ctx context.Context, coll *Collection[T]) error {
		result = coll
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil

}

func (s *GenericStore[T, H]) Delete(ctx context.Context, id string) error {
	return s.Update(ctx, func(_ context.Context, coll *Collection[T]) error {
		// iterate over the rules to find the rule
		for idx, e := range coll.Elements {
			if s.helper.ID(e) == id {
				// delete the rule
				coll.Elements = append(coll.Elements[:idx], coll.Elements[idx+1:]...)

				// return early and save the ruleset
				return nil
			}
		}
		return ErrElementNotFound
	})
}

func (s *GenericStore[T, H]) Upsert(ctx context.Context, elem T, observedGeneration *int64) error {
	return s.Update(ctx, func(_ context.Context, coll *Collection[T]) error {
		// iterate over the store list to find the element with the same idx
		pos := -1
		for idx, e := range coll.Elements {
			if s.helper.ID(e) == s.helper.ID(elem) {
				pos = idx
			}
		}

		// new element required
		if pos == -1 {
			// create a new rule
			coll.Elements = append(coll.Elements, elem)

			// by definition, the generation of a new element is 1
			s.helper.SetGeneration(elem, 1)

			return nil
		}

		// check if there had been a conflicted updated
		storedElem := coll.Elements[pos]
		storedGeneration := s.helper.GetGeneration(storedElem)
		if observedGeneration != nil && *observedGeneration != storedGeneration {
			level.Warn(s.logger).Log(
				"msg", "conflicting update, please try again",
				"observed_generation", observedGeneration,
				"stored_generation", storedGeneration,
			)

			return &ErrConflictGeneration{
				ObservedGeneration: *observedGeneration,
				StoreGeneration:    storedGeneration,
			}
		}

		s.helper.SetGeneration(elem, storedGeneration+1)
		coll.Elements[pos] = elem

		return nil
	})
}

type UpdateTxn[T any] func(context.Context, *Collection[T]) error

// Update will under write lock, call a transaction the Collection. If there is an error returned, the update will be cancelled.
func (s *GenericStore[T, H]) Update(
	ctx context.Context,
	txn UpdateTxn[T],
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
	if err := txn(ctx, data); err != nil {
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
