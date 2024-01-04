package settings

import (
	"context"
	"encoding/json"
	"path"

	"github.com/pkg/errors"
	"github.com/thanos-io/objstore"
	"go.etcd.io/bbolt"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

const (
	settingsFilename = "settings.bolt"
)

func NewPersistentStore(bucket objstore.Bucket, dataPath string) (Store, error) {
	db, err := bbolt.Open(path.Join(dataPath, settingsFilename), 0755, nil)
	if err != nil {
		return nil, err
	}

	store := &persistentStore{
		bucket: bucket,
		db:     db,
	}

	return store, nil
}

type persistentStore struct {
	bucket objstore.Bucket
	db     *bbolt.DB
}

func (s *persistentStore) Get(ctx context.Context, tenantID string) ([]*settingsv1.Setting, error) {
	settings := make([]*settingsv1.Setting, 0)

	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(tenantID))
		if bucket == nil {
			return nil
		}

		err := bucket.ForEach(func(k []byte, v []byte) error {
			setting := &settingsv1.Setting{}
			err := json.Unmarshal(v, setting)
			if err != nil {
				return err
			}

			settings = append(settings, setting)
			return nil
		})
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return settings, nil
}

func (s *persistentStore) Set(ctx context.Context, tenantID string, setting *settingsv1.Setting) (*settingsv1.Setting, error) {
	err := s.db.Update(func(tx *bbolt.Tx) error {
		// Get or create the bucket for this tenant.
		bucket, err := tx.CreateBucketIfNotExists([]byte(tenantID))
		if err != nil {
			return err
		}

		// Check if there is an existing setting with this key.
		value := bucket.Get([]byte(setting.Name))
		if value != nil {
			// An existing setting was found, make sure we don't overwrite it if
			// its newer.
			oldSetting := &settingsv1.Setting{}
			err = json.Unmarshal(value, oldSetting)
			if err != nil {
				return errors.Wrapf(err, "failed to update %s", setting.Name)
			}

			if oldSetting.ModifiedAt > setting.ModifiedAt {
				return errors.Wrapf(oldSettingErr, "failed to update %s", setting.Name)
			}
		}

		// Encode the new setting.
		value, err = json.Marshal(setting)
		if err != nil {
			return errors.Wrapf(err, "failed to update %s", setting.Name)
		}

		// Insert the new setting.
		err = bucket.Put([]byte(setting.Name), value)
		if err != nil {
			return errors.Wrapf(err, "failed to update %s", setting.Name)
		}

		// TODO(bryan) Sync the db with object storage.

		return nil
	})
	if err != nil {
		return nil, err
	}

	return setting, nil
}

func (s *persistentStore) Sync(ctx context.Context) error {
	panic("unimplemented")
}
