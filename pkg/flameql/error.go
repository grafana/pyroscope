package flameql

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidQuerySyntax    = errors.New("invalid query syntax")
	ErrInvalidAppName        = errors.New("invalid application name")
	ErrInvalidMatchersSyntax = errors.New("invalid tag matchers syntax")
	ErrInvalidKey            = errors.New("invalid tag key")
	ErrInvalidValueSyntax    = errors.New("invalid tag value syntax")

	ErrAppNameIsRequired       = errors.New("application name is required")
	ErrMatchOperatorIsRequired = errors.New("tag match operator is required")
	ErrKeyIsRequired           = errors.New("tag key is required")
	ErrKeyReserved             = errors.New("tag key is reserved")

	ErrUnknownOp = errors.New("unknown tag match operator")
)

type Error struct {
	Inner error
	Expr  string
	// TODO: add offset?
}

func newErr(err error, expr string) *Error { return &Error{Inner: err, Expr: expr} }

func (e *Error) Error() string { return e.Inner.Error() + ": " + e.Expr }

func (e *Error) Unwrap() error { return e.Inner }

func newInvalidKeyRuneError(k string, r rune) *Error {
	return newErr(ErrInvalidKey, fmt.Sprintf("%s: character is not allowed: %q", k, r))
}
