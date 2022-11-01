package service

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

var (
	ErrRecordNotFound = errors.New("record not found")
	ErrDatabase       = errors.New("database error")
)

// WrapGormError wraps gorm errors so that clients are not aware of the underlying implementation
// Keep in mind we don't wrap every single error, only the ones that have meaningful information
func WrapGormError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrRecordNotFound
	}

	return fmt.Errorf("%w: %w", ErrDatabase, err)
}
