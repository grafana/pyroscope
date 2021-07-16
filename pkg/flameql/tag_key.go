package flameql

import (
	"unicode"
)

const (
	ReservedTagKeyName = "__name__"
)

var reservedTagKeys = []string{
	ReservedTagKeyName,
}

func ValidateTagKey(k string) error {
	if len(k) == 0 {
		return ErrTagKeyIsRequired
	}
	if IsTagKeyReserved(k) {
		return newErr(ErrTagKeyReserved, k)
	}
	for _, r := range k {
		if !IsTagKeyRuneAllowed(r) {
			return newInvalidTagKeyRuneError(k, r)
		}
	}
	return nil
}

func IsTagKeyRuneAllowed(r rune) bool {
	return unicode.IsDigit(r) || unicode.IsLetter(r) ||
		r == '-' || r == '_' || r == '.' || r == '/'
}

func IsTagKeyReserved(k string) bool {
	for _, s := range reservedTagKeys {
		if s == k {
			return true
		}
	}
	return false
}
