class BikeController < ApplicationController
  def index
    OpenTelemetry.tracer_provider.tracer('my-tracer').in_span("BikeController") do |_|
      helpers.find_nearest_vehicle( 0.4, "bike")
      render html: "Bike ordered"
    end
  end
end
