package storage

import (
	"encoding/json"

	"github.com/dgraph-io/badger/v2"
)

const analyticsKey = "analytics"

func (s *Storage) SaveAnalytics(a interface{}) error {
	v, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return s.main.Update(func(txn *badger.Txn) error {
		return txn.SetEntry(badger.NewEntry([]byte(analyticsKey), v))
	})
}

func (s *Storage) LoadAnalytics(a interface{}) error {
	err := s.main.View(func(txn *badger.Txn) error {
		v, err := txn.Get([]byte(analyticsKey))
		if err != nil {
			return err
		}
		return v.Value(func(val []byte) error {
			return json.Unmarshal(val, a)
		})
	})
	return err
}
