package storage

import (
	"fmt"
	"strings"

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
	rows, err := s.main.Query("SELECT version FROM version")
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if rows.Next() {
		if err := rows.Scan(&version); err != nil {
			return 0, err
		}
	}

	return version, nil
}

func (s *Storage) setDbVersion(v int) error {
	_, err := s.main.Exec("UPDATE version SET version = ?", v)
	return err
}

func migrateDictionaryKeys(s *Storage) error {
	return nil

	appNameKeys := map[string]struct{}{}
	segmentNameKeys := map[string][]byte{}
	rows, err := s.main.Query("SELECT key, value FROM dictionary WHERE key LIKE 'tree.%'")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return err
		}
		if !strings.Contains(key, "{") {
			appNameKeys[key] = struct{}{}
		} else {
			segmentNameKeys[key] = value
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for k, v := range segmentNameKeys {
		dictKey := segment.FromTreeToDictKey(k)
		if _, ok := appNameKeys[dictKey]; ok {
			// The dictionary is most likely incomplete and causes
			// the problem described in the function comment.
			continue
		}
		// Migration from version before 0.0.34.
		_, err := s.main.Exec("INSERT INTO dictionary (key, value) VALUES (?, ?)", dictKey, v)
		if err != nil {
			return err
		}
		// Remove dict stored with old keys.
		_, err = s.main.Exec("DELETE FROM dictionary WHERE key = ?", k)
		if err != nil {
			return err
		}
	}

	return nil
}
