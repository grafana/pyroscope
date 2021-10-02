def find_nearest_vehicle(n, vehicle)
  Pyroscope.tag_wrapper({ "vehicle" => vehicle }) do
    i = 0
    start_time = Time.new
    while Time.new - start_time < n do
      i += 1
    end
  end
end
