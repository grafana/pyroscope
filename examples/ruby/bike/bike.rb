require_relative '../utility/utility'  

def order_bike(search_radius)
  Pyroscope.tag_wrapper({ "vehicle" => "bike" }) do
    find_nearest_vehicle(search_radius)
  end
end