class ScooterController < ApplicationController
    def index
        helpers.find_nearest_vehicle( 0.6, "scooter")
        render html: "Scooter ordered"
    end

end
