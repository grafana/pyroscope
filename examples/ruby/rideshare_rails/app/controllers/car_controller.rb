class CarController < ApplicationController
    def index
        helpers.find_nearest_vehicle( 0.5, "car")
        render html: "Car ordered"
    end
end
