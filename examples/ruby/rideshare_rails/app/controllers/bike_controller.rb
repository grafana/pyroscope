class BikeController < ApplicationController
    def index
        helpers.find_nearest_vehicle( 0.4, "bike")
        render html: "Bike ordered"
    end
end
