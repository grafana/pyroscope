package model

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer/convert"
)

// AdhocProfile describes a profile that is controlled by AdhocService.
type AdhocProfile struct {
	ID        string
	Name      string
	Profile   *flamebearer.FlamebearerProfile
	UpdatedAt time.Time
}

type GetAdhocProfileDiffByIDParams struct {
	BaseID string
	DiffID string
}

type CreateAdhocProfileParams struct {
	Profile convert.ProfileFile
}

type BuildAdhocProfileDiffParams struct {
	Base *flamebearer.FlamebearerProfile
	Diff *flamebearer.FlamebearerProfile
}
