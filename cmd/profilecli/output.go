package main

import (
	"encoding/json"
	"os"

	"github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func outputSeries(result []*typesv1.Labels) error {
	enc := json.NewEncoder(os.Stdout)
	m := make(map[string]interface{})
	for _, s := range result {
		for k := range m {
			delete(m, k)
		}
		for _, l := range s.Labels {
			m[l.Name] = l.Value
		}
		if err := enc.Encode(m); err != nil {
			return err
		}
	}
	return nil
}
