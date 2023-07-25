class Scooter::ScooterController < ActionController::Base
  def show
    OpenTelemetry.tracer_provider.tracer('my-tracer').in_span("ScooterController") do |_|
      ApplicationHelper::find_nearest_vehicle(2, "scooter")
    end
  end
end
