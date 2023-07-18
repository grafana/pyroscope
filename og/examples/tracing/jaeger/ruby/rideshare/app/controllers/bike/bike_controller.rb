class Bike::BikeController < ActionController::Base
  def show
    OpenTelemetry.tracer_provider.tracer('my-tracer').in_span("BikeController") do |_|
      ApplicationHelper::find_nearest_vehicle(1, "bike")
    end
  end
end
