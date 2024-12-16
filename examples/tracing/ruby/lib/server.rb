require "sinatra"
require "thin"
require "pyroscope"
require "pyroscope/otel"
require "opentelemetry-sdk"
require 'opentelemetry-exporter-otlp'
require 'opentelemetry/trace/propagation/trace_context'
require_relative 'scooter/scooter'
require_relative 'bike/bike'
require_relative 'car/car'

app_name = ENV.fetch("PYROSCOPE_APPLICATION_NAME", "rideshare.ruby.push.app")
pyroscope_server_address = ENV.fetch("PYROSCOPE_SERVER_ADDRESS", "http://pyroscope:4040")

Pyroscope.configure do |config|
  config.app_name = app_name
  config.server_address = pyroscope_server_address
  config.tags = {
    "region": ENV["REGION"],
  }
end

OpenTelemetry::SDK.configure do |c|
  c.add_span_processor Pyroscope::Otel::SpanProcessor.new("#{app_name}.cpu", pyroscope_server_address)

  c.add_span_processor(
    OpenTelemetry::SDK::Trace::Export::BatchSpanProcessor.new(
      OpenTelemetry::Exporter::OTLP::Exporter.new(
        endpoint: 'http://tempo:4318/v1/traces'
      )
    )
  )

end

# Extract trace context from load generator requests to link our handler spans with the parent
# load generator trace, creating a complete distributed trace across both services.
before do
  if (traceparent = request.env['HTTP_TRACEPARENT'])
    # Parse traceparent: version-traceid-spanid-flags
    _version, trace_id_hex, parent_span_id_hex, _flags = traceparent.split('-')

    # Get the propagator and carrier
    carrier = { 'traceparent' => traceparent }

    # Extract context using the propagator
    @extracted_context = OpenTelemetry.propagation.extract(carrier)
  end
end

tracer = OpenTelemetry.tracer_provider.tracer('my-tracer')

get "/bike" do
  OpenTelemetry::Context.with_current(@extracted_context) do
    tracer.in_span("BikeHandler") do |span|
      order_bike(0.4)
      "<p>Bike ordered</p>"
    end
  end
end

get "/scooter" do
  OpenTelemetry::Context.with_current(@extracted_context) do
    tracer.in_span("ScooterHandler") do |span|
      order_scooter(0.6)
      "<p>scooter ordered</p>"
    end
  end
end

get "/car" do
  OpenTelemetry::Context.with_current(@extracted_context) do
    tracer.in_span("CarHandler") do |span|
      order_car(0.8)
      "<p>car ordered</p>"
    end
  end
end

set :bind, '0.0.0.0'
set :port, ENV["RIDESHARE_LISTEN_PORT"] || 5000

run Sinatra::Application.run!
