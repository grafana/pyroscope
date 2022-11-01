package model

import "errors"

var (
	ErrApplicationNotFound  = NotFoundError{errors.New("application not found")}
	ErrApplicationNameEmpty = ValidationError{errors.New("application name can't be empty")}
)

func ValidateAppName(appName string) error {
	// TODO(eh-am): check for invalid characters, min/max names?
	// problem is that it needs to be in sync with existing data
	if appName == "" {
		return ErrApplicationNameEmpty
	}
	return nil
}
