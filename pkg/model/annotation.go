package model

import (
	"errors"
	"time"

	"github.com/hashicorp/go-multierror"
)

var (
	ErrAnnotationInvalidAppName   = errors.New("invalid app name")
	ErrAnnotationInvalidTimestamp = errors.New("invalid timestamp")
	ErrAnnotationInvalidContent   = errors.New("invalid content")
)

type Annotation struct {
	AppName   string
	Content   string
	Timestamp time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CreateAnnotation struct {
	AppName   string
	Content   string
	Timestamp time.Time
}

func (a CreateAnnotation) Validate() error {
	var err error

	if a.AppName == "" {
		err = multierror.Append(err, ErrAnnotationInvalidAppName)
	}

	if a.Content == "" {
		err = multierror.Append(err, ErrAnnotationInvalidContent)
	}

	if a.Timestamp.IsZero() {
		err = multierror.Append(err, ErrAnnotationInvalidTimestamp)
	}

	return err
}
