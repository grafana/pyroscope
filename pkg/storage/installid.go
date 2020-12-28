package storage

import (
	"github.com/dgraph-io/badger/v2"
	"github.com/google/uuid"
)

const installID = "installID"

func (s *Storage) InstallID() string {
	var id []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(installID))

		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}
			return err
		}

		err = item.Value(func(val []byte) error {
			id = append([]byte{}, val...)
			return nil
		})
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return "id-read-error"
	}

	if id == nil {
		id = []byte(newID())
		err = s.db.Update(func(txn *badger.Txn) error {
			return txn.SetEntry(badger.NewEntry([]byte(installID), id))
		})
		if err != nil {
			return "id-write-error"
		}
	}

	return string(id)
}

func newID() string {
	return uuid.New().String()
}
