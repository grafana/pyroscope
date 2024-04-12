class ScooterController < ApplicationController
  def index
    OpenTelemetry.tracer_provider.tracer('my-tracer').in_span("ScooterController") do |_|
      helpers.find_nearest_vehicle( 0.6, "scooter")
      render html: "Scooter ordered"
    end
  end
end
