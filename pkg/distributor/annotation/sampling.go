package annotation

import (
	"encoding/json"

	"github.com/grafana/pyroscope/pkg/distributor/sampling"
)

type SampledAnnotation struct {
	Source *sampling.Source `json:"source"`
}

func CreateProfileAnnotation(source *sampling.Source) ([]byte, error) {
	a := &ProfileAnnotation{
		Body: SampledAnnotation{
			Source: source,
		},
	}
	return json.Marshal(a)
}
