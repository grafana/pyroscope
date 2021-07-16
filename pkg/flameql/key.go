package flameql

import (
	"unicode"
)

const (
	ReservedKeyName = "__name__"
)

var reservedKeys = []string{
	ReservedKeyName,
}

func ValidateKey(k string) error {
	if len(k) == 0 {
		return ErrKeyIsRequired
	}
	if IsKeyReserved(k) {
		return newErr(ErrKeyReserved, k)
	}
	for _, r := range k {
		if !IsKeyRuneAllowed(r) {
			return newInvalidKeyRuneError(k, r)
		}
	}
	return nil
}

func IsKeyRuneAllowed(r rune) bool {
	return unicode.IsDigit(r) || unicode.IsLetter(r) ||
		r == '-' || r == '_' || r == '.' || r == '/'
}

func IsKeyReserved(k string) bool {
	for _, s := range reservedKeys {
		if s == k {
			return true
		}
	}
	return false
}
