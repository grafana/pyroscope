class ScooterController < ApplicationController
  def index
    helpers.find_nearest_vehicle( 0.6, "scooter")
    i = 0; while i < MULTIPLIER * 3; i += 1; end
    logger_debug "Scooter ordered"
    render html: "Scooter ordered"
  end
end
