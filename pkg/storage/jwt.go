package storage

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const (
	jwtLength = 32
	jwtSecret = "jwtSecret"
)

func (s *Storage) JWT() (string, error) {
	var secret string
	row, err := s.main.Query("SELECT secret FROM "+jwtSecret+" WHERE id = ?", 1)
	if err != nil {
		return "", err
	}
	defer row.Close()
	if row.Next() {
		if err := row.Scan(&secret); err != nil {
			return "", err
		}
	}
	if secret == "" {
		generatedJWT, err := newJWTSecret()
		if err != nil {
			return "", err
		}
		secret = generatedJWT
		query := fmt.Sprintf("INSERT INTO %s (id, secret) VALUES (?, ?)", jwtSecret)
		_, err = s.main.Exec(query, 1, secret)
		if err != nil {
			return "", err
		}
	}
	return secret, nil
}

func newJWTSecret() (string, error) {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-"
	ret := make([]byte, jwtLength)
	for i := 0; i < jwtLength; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		ret[i] = letters[num.Int64()]
	}

	return string(ret), nil
}
