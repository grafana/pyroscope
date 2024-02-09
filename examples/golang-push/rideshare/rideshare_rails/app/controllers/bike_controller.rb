class BikeController < ApplicationController
  def index
    helpers.find_nearest_vehicle( 0.4, "bike")
    i = 0; while i < MULTIPLIER * 3; i += 1; end
    logger_debug "Bike ordered"
    render html: "Bike ordered"
  end
end
