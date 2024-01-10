package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/thanos-io/objstore"
	"golang.org/x/exp/slices"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

var (
	oldSettingErr    = errors.New("newer update already written")
	settingsFilename = "tenant_settings.json"
)

// NewMemoryStore will create a settings store with an in-memory objstore
// bucket.
func NewMemoryStore() (Store, error) {
	return NewBucketStore(objstore.NewInMemBucket())
}

// NewBucketStore will create a settings store with an objstore bucket.
func NewBucketStore(bucket objstore.Bucket) (Store, error) {
	store := &bucketStore{
		store:  make(map[string]map[string]*settingsv1.Setting),
		bucket: bucket,
	}

	return store, nil
}

type bucketStore struct {
	rw sync.Mutex

	// store is kv pairs, indexed first by tenant id.
	store map[string]map[string]*settingsv1.Setting

	// bucket is an object store bucket.
	bucket objstore.Bucket
}

func (s *bucketStore) Get(ctx context.Context, tenantID string) ([]*settingsv1.Setting, error) {
	s.rw.Lock()
	defer s.rw.Unlock()

	err := s.unsafeLoad(ctx)
	if err != nil {
		return nil, err
	}

	tenantSettings := s.store[tenantID]

	settings := make([]*settingsv1.Setting, 0, len(s.store[tenantID]))
	for _, setting := range tenantSettings {
		settings = append(settings, setting)
	}

	slices.SortFunc(settings, func(a, b *settingsv1.Setting) int {
		return strings.Compare(a.Name, b.Name)
	})
	return settings, nil
}

func (s *bucketStore) Set(ctx context.Context, tenantID string, setting *settingsv1.Setting) (*settingsv1.Setting, error) {
	s.rw.Lock()
	defer s.rw.Unlock()

	err := s.unsafeLoad(ctx)
	if err != nil {
		return nil, err
	}

	_, ok := s.store[tenantID]
	if !ok {
		s.store[tenantID] = make(map[string]*settingsv1.Setting, 1)
	}

	oldSetting, ok := s.store[tenantID][setting.Name]
	if ok && oldSetting.ModifiedAt > setting.ModifiedAt {
		return nil, errors.Wrapf(oldSettingErr, "failed to update %s", setting.Name)
	}
	s.store[tenantID][setting.Name] = setting

	err = s.unsafeFlush(ctx)
	if err != nil {
		return nil, err
	}

	return setting, nil
}

func (s *bucketStore) Flush(ctx context.Context) error {
	s.rw.Lock()
	defer s.rw.Unlock()

	return s.unsafeFlush(ctx)
}

func (s *bucketStore) Close() error {
	return s.bucket.Close()
}

// unsafeFlush will flush the store to object storage. This is not thread-safe,
// the store's write mutex should be acquired first.
func (s *bucketStore) unsafeFlush(ctx context.Context) error {
	data, err := json.Marshal(s.store)
	if err != nil {
		return err
	}

	err = s.bucket.Upload(ctx, settingsFilename, bytes.NewReader(data))
	if err != nil {
		return err
	}
	return nil
}

// unsafeLoad will read the store in object storage into memory, if it exists.
// This is not thread-safe, the store's write mutex should be acquired first.
func (s *bucketStore) unsafeLoad(ctx context.Context) error {
	reader, err := s.bucket.Get(ctx, settingsFilename)
	if err != nil {
		if s.bucket.IsObjNotFoundErr(err) {
			// It is OK if we don't find the file.
			return nil
		}
		return err
	}

	err = json.NewDecoder(reader).Decode(&s.store)
	if err != nil {
		return err
	}

	err = reader.Close()
	if err != nil {
		return err
	}
	return nil
}
