package storage

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/dgraph-io/badger/v2"

	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

var migrations = []migration{
	migrateDictionaryKeys,
}

type migration func(*Storage) error

const dbVersionKey = "db-version"

func (s *Storage) migrate() error {
	ver, err := s.dbVersion()
	if err != nil {
		return err
	}
	switch {
	case ver == len(migrations):
		return nil
	case ver > len(migrations):
		return fmt.Errorf("db version %d: future versions are not supported", ver)
	}
	for v, m := range migrations[ver:] {
		if err = m(s); err != nil {
			return fmt.Errorf("migration %d: %w", v, err)
		}
	}
	return s.setDbVersion(len(migrations))
}

// dbVersion returns the number of migrations applied to the storage.
func (s *Storage) dbVersion() (int, error) {
	var version int
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(dbVersionKey))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			version, err = strconv.Atoi(string(val))
			return err
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return 0, nil
	}
	return version, err
}

func (s *Storage) setDbVersion(v int) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.SetEntry(&badger.Entry{
			Key:   []byte(dbVersionKey),
			Value: []byte(strconv.Itoa(v)),
		})
	})
}

const dictionaryKeyPrefix = "d:"

func toDictKey(k string) []byte { return []byte(dictionaryKeyPrefix + k) }

// In 0.0.34 we changed dictionary key format from normalized segment key
// (e.g, app.name{foo=bar}) to just app name. See e756a200a for details.
// On deserialization, when a dictionary is loaded from disk to cache, the
// logic was to check both keys: if app name key exists, use the found
// dictionary, otherwise lookup the dictionary using normalized segment key.
// The problem is that the check never reported false, returning an empty
// dictionary instead. Thus, dictionaries created in 0.0.34 and 0.0.35 may be
// incomplete, which results in "label not found" nodes in rendered trees.
//
// Depending on the version, migration has different impact:
//  * < 0.0.34:
//  	Dictionary keys to be renamed to app name format. No negative impact.
//  * > 0.0.33:
//  	No impact. Data ingested prior to the update to 0.0.34/0.0.35 is
//  	corrupted, which results in "label not found" nodes.
func migrateDictionaryKeys(s *Storage) error {
	appNameKeys := map[string]struct{}{}
	segmentNameKeys := map[string][]byte{}
	return s.dbDicts.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(dictionaryKeyPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()
		// Find all dicts with keys:
		//  - in normalized segment key format.
		//  - in application name format.
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := item.Key()
			item.ExpiresAt()
			if len(k) < len(dictionaryKeyPrefix) {
				continue
			}
			k = k[len(dictionaryKeyPrefix):]
			// Make sure the dictionary is valid.
			b, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			d, err := dict.FromBytes(b)
			if err != nil {
				return err
			}
			if d == nil {
				continue
			}
			if !strings.Contains(string(k), "{") {
				appNameKeys[string(k)] = struct{}{}
			} else {
				segmentNameKeys[string(k)] = b
			}
		}

		for k, v := range segmentNameKeys {
			dictKey := segment.FromTreeToDictKey(k)
			if _, ok := appNameKeys[dictKey]; ok {
				// The dictionary is most likely incomplete and causes
				// the problem described in the function comment.
				continue
			}
			// Migration from version before 0.0.34.
			if err := txn.Set(toDictKey(dictKey), v); err != nil {
				return err
			}
			// Remove dict stored with old keys.
			if err := txn.Delete(toDictKey(k)); err != nil {
				return err
			}
		}

		return nil
	})
}
