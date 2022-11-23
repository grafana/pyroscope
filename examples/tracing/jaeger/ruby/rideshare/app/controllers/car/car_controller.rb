class Car::CarController < ActionController::Base
  def show
    OpenTelemetry.tracer_provider.tracer('my-tracer').in_span("CarController") do |_|
      ApplicationHelper::find_nearest_vehicle(3, "car")
    end
  end
end
