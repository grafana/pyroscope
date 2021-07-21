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

// ValidateTagKey report an error if the given key k violates constraints.
//
// The function should be used to validate user input. The function returns
// ErrTagKeyReserved if the key is valid but reserved for internal use.
func ValidateTagKey(k string) error {
	if len(k) == 0 {
		return ErrTagKeyIsRequired
	}
	for _, r := range k {
		if !IsTagKeyRuneAllowed(r) {
			return newInvalidTagKeyRuneError(k, r)
		}
	}
	if IsTagKeyReserved(k) {
		return newErr(ErrTagKeyReserved, k)
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
