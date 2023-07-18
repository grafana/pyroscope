package model

import (
	"fmt"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/pyroscope-io/pyroscope/pkg/flameql"
)

var (
	ErrAnnotationInvalidAppName   = ValidationError{errors.New("invalid app name")}
	ErrAnnotationInvalidTimestamp = ValidationError{errors.New("invalid timestamp")}
	ErrAnnotationInvalidContent   = ValidationError{errors.New("invalid content")}
)

type Annotation struct {
	AppName   string    `gorm:"index:idx_appname_timestamp,unique;not null;default:null"`
	Timestamp time.Time `gorm:"index:idx_appname_timestamp,unique;not null;default:null"`
	Content   string    `gorm:"not null;default:null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CreateAnnotation struct {
	AppName   string
	Content   string
	Timestamp time.Time
}

// Parse parses and validates
// It adds a default timestamp (to time.Now) if not present
// And check required fields are set
func (a *CreateAnnotation) Parse() error {
	var err error

	if a.Timestamp.IsZero() {
		a.Timestamp = time.Now()
	}

	if a.AppName == "" {
		err = multierror.Append(err, ErrAnnotationInvalidAppName)
	} else {
		if parseErr := a.handleAppNameQuery(); parseErr != nil {
			err = multierror.Append(err, parseErr)
		}
	}

	if a.Content == "" {
		err = multierror.Append(err, ErrAnnotationInvalidContent)
	}

	return err
}

func (a *CreateAnnotation) handleAppNameQuery() error {
	if a.AppName == "" {
		return ErrAnnotationInvalidAppName
	}

	// Strip labels
	key, parseErr := flameql.ParseQuery(a.AppName)
	if parseErr != nil {
		return fmt.Errorf("%w: %v", ErrAnnotationInvalidAppName, parseErr)
	}

	// Handle when only tags are passed
	if key == nil || key.AppName == "" {
		return ErrAnnotationInvalidAppName
	}

	a.AppName = key.AppName

	return nil
}
