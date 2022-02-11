module rideshare

go 1.17

require (
	github.com/pyroscope-io/client v0.0.0-20211206204731-3fd0a4b8239c
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.28.0
	go.opentelemetry.io/otel v1.3.0
	go.opentelemetry.io/otel/exporters/jaeger v1.3.0
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.3.0
	go.opentelemetry.io/otel/sdk v1.3.0
	go.opentelemetry.io/otel/trace v1.3.0
	golang.org/x/sys v0.0.0-20210510120138-977fb7262007 // indirect
)
