require_relative '../utility/utility' 

def order_car(search_radius)
  Pyroscope.tag_wrapper({ "vehicle" => "car" }) do
    find_nearest_vehicle(search_radius)
  end
end