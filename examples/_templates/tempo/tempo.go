package tempo

import (
	"embed"

	templates "github.com/grafana/pyroscope/examples/_templates"
)

//go:embed assets/*
var f embed.FS

func init() {
	templates.Add(&templates.Template{
		Name:         "tempo",
		Description:  "Tempo is a distributed tracing system.",
		Assets:       f,
		CleanUpPaths: []string{"tempo"},
		Destinations: []string{
			"examples/tracing/dotnet",
			"examples/tracing/golang",
			"examples/tracing/java",
			"examples/tracing/python",
			"examples/tracing/ruby",
			"examples/tracing/tempo",
		},
	})
}
