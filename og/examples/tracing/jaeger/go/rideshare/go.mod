module rideshare

go 1.17

require (
	github.com/grafana/pyroscope-go v1.0.2
	github.com/pyroscope-io/otel-profiling-go v0.4.0
	github.com/sirupsen/logrus v1.9.3
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.44.0
	go.opentelemetry.io/otel v1.18.0
	go.opentelemetry.io/otel/exporters/jaeger v1.17.0
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.18.0
	go.opentelemetry.io/otel/sdk v1.18.0
	go.opentelemetry.io/otel/trace v1.18.0
)

require (
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/grafana/pyroscope-go/godeltaprof v0.1.4 // indirect
	go.opentelemetry.io/otel/metric v1.18.0 // indirect
	golang.org/x/sys v0.12.0 // indirect
)
