require_relative '../utility/utility'

def order_scooter(search_radius)
  Pyroscope.tag_wrapper({ "vehicle" => "scooter" }) do
    find_nearest_vehicle(search_radius)
  end
end