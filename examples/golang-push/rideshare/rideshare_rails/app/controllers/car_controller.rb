class CarController < ApplicationController
  def index
    helpers.find_nearest_vehicle(0.5, "car")
    i = 0; while i < MULTIPLIER * 3; i += 1; end
    logger_debug "Car ordered"
    render html: "Car ordered"
  end
end
