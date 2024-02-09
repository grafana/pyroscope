require 'pyroscope/otel'

app_name = ENV.fetch("PYROSCOPE_APPLICATION_NAME", "rails-ride-sharing-app")
pyroscope_server_address = ENV.fetch("PYROSCOPE_SERVER_ADDRESS", "http://pyroscope:4040")
jaeger_endpoint = ENV.fetch("JAEGER_ENDPOINT", "http://localhost:14268/api/traces")

Pyroscope.configure do |config|
  config.app_name       = app_name
  config.server_address = pyroscope_server_address
  config.auth_token     = ENV.fetch("PYROSCOPE_AUTH_TOKEN", "")

  config.tags = {
    "region": ENV["REGION"] || "us-east",
    "compression": ENV["COMPRESSION"],
  }
end

OpenTelemetry::SDK.configure do |c|
  c.service_name = app_name
  c.add_span_processor Pyroscope::Otel::SpanProcessor.new("#{app_name}.cpu", pyroscope_server_address)
  c.add_span_processor OpenTelemetry::SDK::Trace::Export::BatchSpanProcessor.new(
    OpenTelemetry::Exporter::Jaeger::CollectorExporter.new(endpoint: jaeger_endpoint))
  c.use_all()
end
