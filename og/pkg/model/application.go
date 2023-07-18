package model

import (
	"errors"

	"github.com/pyroscope-io/pyroscope/pkg/flameql"
)

var (
	ErrApplicationNotFound = NotFoundError{errors.New("application not found")}
)

func ValidateAppName(appName string) error {
	err := flameql.ValidateAppName(appName)
	if err != nil {
		return ValidationError{err}
	}
	return nil
}
