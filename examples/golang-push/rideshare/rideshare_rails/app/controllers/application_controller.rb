class ApplicationController < ActionController::Base
  include Logging::LoggingHelper

  around_action :trace_action

  private

  def trace_action
    Pyroscope.tag_wrapper({ "vehicle" => action_name }) do
      OpenTelemetry.tracer_provider.tracer('my-tracer').in_span(controller_name.classify) do |_|
        yield
      end
    end
  end
end
