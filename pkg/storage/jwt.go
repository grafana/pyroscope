package storage

import (
	"crypto/rand"
	"math/big"

	"github.com/dgraph-io/badger/v2"
)

const (
	jwtLenght = 32
	jwtSecret = "jwtSecret"
)

func (s *Storage) JWT() (string, error) {
	var secret []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(jwtSecret))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}
			return err
		}

		err = item.Value(func(val []byte) error {
			secret = append([]byte{}, val...)
			return nil
		})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	if secret == nil {
		generatedJWT, err := newJWTSecret()
		if err != nil {
			return "", err
		}
		secret = []byte(generatedJWT)
		err = s.db.Update(func(txn *badger.Txn) error {
			return txn.SetEntry(badger.NewEntry([]byte(jwtSecret), secret))
		})
		if err != nil {
			return "", err
		}
	}

	return string(secret), nil
}

func newJWTSecret() (string, error) {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-"
	ret := make([]byte, jwtLenght)
	for i := 0; i < jwtLenght; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		ret[i] = letters[num.Int64()]
	}

	return string(ret), nil
}
