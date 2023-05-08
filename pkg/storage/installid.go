package storage

import (
	"database/sql"

	"github.com/google/uuid"
)

const installID = "installID"

// func (s *Storage) InstallID() string {
// 	var id []byte
// 	err := s.main.View(func(txn *badger.Txn) error {
// 		item, err := txn.Get([]byte(installID))
// 		if err != nil {
// 			if err == badger.ErrKeyNotFound {
// 				return nil
// 			}
// 			return err
// 		}

// 		err = item.Value(func(val []byte) error {
// 			id = append([]byte{}, val...)
// 			return nil
// 		})
// 		if err != nil {
// 			return err
// 		}
// 		return nil
// 	})
// 	if err != nil {
// 		return "id-read-error"
// 	}

// 	if id == nil {
// 		id = []byte(newID())
// 		err = s.main.Update(func(txn *badger.Txn) error {
// 			return txn.SetEntry(badger.NewEntry([]byte(installID), id))
// 		})
// 		if err != nil {
// 			return "id-write-error"
// 		}
// 	}

// 	return string(id)
// }

// func newID() string {
// 	return uuid.New().String()
// }

func (s *Storage) InstallID() string {
	var id []byte
	rows, err := s.main.Query("SELECT value FROM storage WHERE key = ?", installID)
	if err != nil {
		if err == sql.ErrNoRows {
			id = []byte(newID())
			_, err := s.main.Exec("INSERT INTO storage (key, value) VALUES (?, ?)", installID, id)
			if err != nil {
				return "id-write-error"
			}
		} else {
			return "id-read-error"
		}
	} else {
		defer rows.Close()
		if rows.Next() {
			if err := rows.Scan(&id); err != nil {
				return "id-read-error"
			}
		} else {
			id = []byte(newID())
			_, err := s.main.Exec("INSERT INTO storage (key, value) VALUES (?, ?)", installID, id)
			if err != nil {
				return "id-write-error"
			}
		}
	}

	return string(id)

}

func newID() string {
	return uuid.New().String()
}
