require "pyroscope"
require "pyroscope/otel"

require "opentelemetry/sdk"
require "opentelemetry/exporter/otlp"

Pyroscope.configure do |config|
  config.application_name = "rideshare.ruby"
  config.log_level = "debug"
  config.server_address = ENV["PYROSCOPE_SERVER_ADDRESS"] || "http://localhost:4040"
end

OpenTelemetry::SDK.configure do |c|
  c.service_name = "rideshare.ruby"
  c.add_span_processor Pyroscope::Otel::SpanProcessor.new("rideshare.ruby.cpu", "http://localhost:4040")
  c.use_all()
end


# if defined?(::Rails::Engine)
#   class Engine < ::Rails::Engine
#     config.after_initialize do
#       puts "huihuihui"
#     end
#   end
# end