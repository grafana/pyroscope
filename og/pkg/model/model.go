package model

import "errors"

type NotFoundError struct{ Err error }

func (e NotFoundError) Error() string { return e.Err.Error() }

func (e NotFoundError) Unwrap() error { return e.Err }

func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var v NotFoundError
	return errors.As(err, &v)
}

type ValidationError struct{ Err error }

func (e ValidationError) Error() string { return e.Err.Error() }

func (e ValidationError) Unwrap() error { return e.Err }

func IsValidationError(err error) bool {
	if err == nil {
		return false
	}
	var v ValidationError
	return errors.As(err, &v)
}

type AuthenticationError struct{ Err error }

func (e AuthenticationError) Error() string { return e.Err.Error() }

func (e AuthenticationError) Unwrap() error { return e.Err }

func IsAuthenticationError(err error) bool {
	if err == nil {
		return false
	}
	var v AuthenticationError
	return errors.As(err, &v)
}

type AuthorizationError struct{ Err error }

func (e AuthorizationError) Error() string { return e.Err.Error() }

func (e AuthorizationError) Unwrap() error { return e.Err }

func IsAuthorizationError(err error) bool {
	if err == nil {
		return false
	}
	var v AuthorizationError
	return errors.As(err, &v)
}

func String(s string) *string { return &s }
