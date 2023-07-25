class CarController < ApplicationController
  def index
    OpenTelemetry.tracer_provider.tracer('my-tracer').in_span("CarController") do |_|
      helpers.find_nearest_vehicle(0.5, "car")
      render html: "Car ordered"
    end
  end
end
