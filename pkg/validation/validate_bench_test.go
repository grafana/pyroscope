package validation

import (
	"fmt"
	"testing"

	"github.com/go-kit/log"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func BenchmarkValidateLabels(b *testing.B) {
	overrides := MockDefaultOverrides()
	logger := log.NewNopLogger()
	labels := []*typesv1.LabelPair{
		{Name: "__name__", Value: "process_cpu"},
		{Name: "service_name", Value: "my-service"},
	}
	for i := 0; i < 18; i++ {
		labels = append(labels, &typesv1.LabelPair{
			Name:  fmt.Sprintf("label_%d", i),
			Value: fmt.Sprintf("value_%d", i),
		})
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ValidateLabels(overrides, "tenant-1", labels, logger)
		if err != nil {
			b.Fatal(err)
		}
	}
}
