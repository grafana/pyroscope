package httputils

import "errors"

type JSONError struct {
	Err error
}

func (e JSONError) Error() string { return e.Err.Error() }

func (e JSONError) Unwrap() error { return e.Err }

func IsJSONError(err error) bool {
	if err == nil {
		return false
	}
	var v JSONError
	return errors.As(err, &v)
}
