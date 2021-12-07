package model

import "errors"

type NotFoundError struct{ Err error }

func (e NotFoundError) Error() string { return e.Err.Error() }

func (e NotFoundError) Unwrap() error { return e.Err }

func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var v *NotFoundError
	return errors.As(err, &v)
}

type ValidationError struct{ Err error }

func (e ValidationError) Error() string { return e.Err.Error() }

func (e ValidationError) Unwrap() error { return e.Err }

func IsValidationError(err error) bool {
	if err == nil {
		return false
	}
	var v *ValidationError
	return errors.As(err, &v)
}
