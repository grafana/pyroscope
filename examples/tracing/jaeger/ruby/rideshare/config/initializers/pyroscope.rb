require "pyroscope"
require "pyroscope/otel"

require "opentelemetry/sdk"
require "opentelemetry/exporter/otlp"
require 'opentelemetry/exporter/jaeger'

app_name = "ride-sharing-app-ruby"
pyroscope_server_address = ENV["PYROSCOPE_SERVER_ADDRESS"] || "http://localhost:4040"
pyroscope_endpoint = ENV["OTEL_PYROSCOPE_ENDPOINT"] || "http://localhost:4040"
jaeger_endpoint = ENV["JAEGER_ENDPOINT"] || "http://localhost:14268/api/traces"

puts("jaeger endpoint #{jaeger_endpoint}")

Pyroscope.configure do |config|
  config.application_name = app_name
  config.log_level = "debug"
  config.server_address = pyroscope_server_address
end

OpenTelemetry::SDK.configure do |c|
  c.service_name = app_name
  c.add_span_processor Pyroscope::Otel::SpanProcessor.new("#{app_name}.cpu", pyroscope_endpoint)
  c.add_span_processor OpenTelemetry::SDK::Trace::Export::BatchSpanProcessor.new(
    OpenTelemetry::Exporter::Jaeger::CollectorExporter.new(endpoint: jaeger_endpoint))
  c.use_all()
end
