require "sinatra"
require "thin"
require "pyroscope"
require_relative 'scooter/scooter'  
require_relative 'bike/bike'
require_relative 'car/car'


Pyroscope.configure do |config|
  config.application_name = "ride-sharing-app"
  config.server_address = "http://pyroscope:4040"
  config.tags = {
    "region": ENV["REGION"],
  }
end

get "/bike" do
  order_bike(0.4)
  "<p>Bike ordered</p>"
end

get "/scooter" do
  order_scooter(0.6)
  "<p>Scooter ordered</p>"
end

get "/car" do
  order_car(0.8)
  "<p>Car ordered</p>"
end


set :bind, '0.0.0.0'
set :port, ENV["RIDESHARE_LISTEN_PORT"] || 5000

run Sinatra::Application.run!
