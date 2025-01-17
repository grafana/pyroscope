package labelset

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidServiceName    = errors.New("invalid service name")
	ErrInvalidLabelSet       = errors.New("invalid label set")
	ErrInvalidLabelName      = errors.New("invalid label name")
	ErrServiceNameIsRequired = errors.New("service name is required")
	ErrLabelNameIsRequired   = errors.New("label name is required")
	ErrLabelNameReserved     = errors.New("label name is reserved")
)

const ReservedLabelNameName = "__name__"

var reservedLabelNames = []string{
	ReservedLabelNameName,
}

type Error struct {
	Inner error
	Expr  string
	// TODO: add offset?
}

func newErr(err error, expr string) *Error { return &Error{Inner: err, Expr: expr} }

func (e *Error) Error() string { return e.Inner.Error() + ": " + e.Expr }

func (e *Error) Unwrap() error { return e.Inner }

func newInvalidLabelNameRuneError(k string, r rune) *Error {
	return newInvalidRuneError(ErrInvalidLabelName, k, r)
}

func NewInvalidServiceNameRuneError(k string, r rune) *Error {
	return newInvalidRuneError(ErrInvalidServiceName, k, r)
}

func newInvalidRuneError(err error, k string, r rune) *Error {
	return newErr(err, fmt.Sprintf("%s: character is not allowed: %q", k, r))
}

// ValidateLabelName report an error if the given key k violates constraints.
//
// The function should be used to validate user input. The function returns
// ErrLabelNameReserved if the key is valid but reserved for internal use.
func ValidateLabelName(k string) error {
	if len(k) == 0 {
		return ErrLabelNameIsRequired
	}
	for _, r := range k {
		if !IsLabelNameRuneAllowed(r) {
			return newInvalidLabelNameRuneError(k, r)
		}
	}
	if IsLabelNameReserved(k) {
		return newErr(ErrLabelNameReserved, k)
	}
	return nil
}

// ValidateServiceName report an error if the given app name n violates constraints.
func ValidateServiceName(n string) error {
	if len(n) == 0 {
		return ErrServiceNameIsRequired
	}
	for _, r := range n {
		if !IsServiceNameRuneAllowed(r) {
			return NewInvalidServiceNameRuneError(n, r)
		}
	}
	return nil
}

func IsLabelNameRuneAllowed(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '.'
}

func IsServiceNameRuneAllowed(r rune) bool {
	return r == '-' || r == '.' || r == '/' || IsLabelNameRuneAllowed(r)
}

func IsLabelNameReserved(k string) bool {
	for _, s := range reservedLabelNames {
		if s == k {
			return true
		}
	}
	return false
}
