package model

import (
	"errors"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer/convert"
)

var (
	ErrAdhocProfileNotFound = NotFoundError{errors.New("profile not found")}
)

// AdhocProfile describes a profile that is controlled by AdhocService.
type AdhocProfile struct {
	ID        string
	Name      string
	UpdatedAt time.Time
}

type GetAdhocProfileDiffByIDParams struct {
	BaseID string
	DiffID string
}

type UploadAdhocProfileParams struct {
	Profile convert.ProfileFile
}
