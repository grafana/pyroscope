package storage

import (
	"encoding/json"
	"errors"
)

const analyticsKey = "analytics"

// func (s *Storage) SaveAnalytics(a interface{}) error {
// 	v, err := json.Marshal(a)
// 	if err != nil {
// 		return err
// 	}
// 	return s.main.Update(func(txn *badger.Txn) error {
// 		return txn.SetEntry(badger.NewEntry([]byte(analyticsKey), v))
// 	})

// }

// func (s *Storage) LoadAnalytics(a interface{}) error {
// 	err := s.main.View(func(txn *badger.Txn) error {
// 		v, err := txn.Get([]byte(analyticsKey))
// 		if err != nil {
// 			return err
// 		}
// 		return v.Value(func(val []byte) error {
// 			return json.Unmarshal(val, a)
// 		})
// 	})
// 	return err
// }

func (s *Storage) SaveAnalytics(a interface{}) error {
	v, err := json.Marshal(a)
	if err != nil {
		return err
	}
	_, err = s.main.Exec("INSERT INTO analytics (key, value) VALUES (?, ?)", analyticsKey, string(v))
	return err
}

func (s *Storage) LoadAnalytics(a interface{}) error {
	rows, err := s.main.Query("SELECT value FROM analytics WHERE key = ?", analyticsKey)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return err
		}
		return json.Unmarshal([]byte(value), a)
	}
	return errors.New("no analytics found")
}
