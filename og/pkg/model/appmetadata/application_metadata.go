package appmetadata

import "github.com/pyroscope-io/pyroscope/pkg/storage/metadata"

type ApplicationMetadata struct {
	// Fully Qualified Name. Eg app.cpu ({__name__}.{profile_type})
	FQName string `gorm:"index,unique;not null;default:null" json:"name"`

	SpyName         string                   `json:"spyName,omitempty"`
	SampleRate      uint32                   `json:"sampleRate,omitempty"`
	Units           metadata.Units           `json:"units,omitempty"`
	AggregationType metadata.AggregationType `json:"-"`
}
