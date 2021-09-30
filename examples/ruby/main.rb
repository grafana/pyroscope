require "sinatra"
require "thin"
require "pyroscope"


Pyroscope.configure do |config|
  config.app_name = "ride-sharing-app"
  config.server_address = "http://pyroscope:4040"
  config.tags = {
    "region": ENV["REGION"],
  }
end

def work(n)
  i = 0
  start_time = Time.new
  while Time.new - start_time < n do
    i += 1
  end
end

def order_bike(n)
  Pyroscope.tag_wrapper({ "vehicle" => "bike" }) do
    work(n)
  end
end

def order_scooter(n)
  Pyroscope.tag_wrapper({ "vehicle" => "scooter" }) do
    work(n)
  end
end

def order_car(n)
  Pyroscope.tag_wrapper({ "vehicle" => "car" }) do
    work(n)
  end
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
set :port, 5000

run Sinatra::Application.run!
