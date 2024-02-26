require "pyroscope"

module ApplicationHelper
  def mutex_lock(n)
    i = 0
    start_time = Time.new
    while Time.new - start_time < n * 10 do
      i += 1
    end
  end

  def check_driver_availability(n)
    i = 0
    start_time = Time.new
    while Time.new - start_time < n / 2 do
      i += 1
    end

    # Every 4 minutes this will artificially create make requests in eu-north region slow
    # this is just for demonstration purposes to show how performance impacts show up in the
    # flamegraph
    current_time = Time.now
    current_minute = current_time.strftime('%M').to_i
    force_mutex_lock = (current_minute * 4 % 8) == 0

    mutex_lock(n) if ENV["REGION"] == "eu-north" and force_mutex_lock
  end

  def find_nearest_vehicle(n, vehicle)
    Pyroscope.tag_wrapper({ "vehicle" => vehicle }) do
      i = 0
      start_time = Time.new
      while Time.new - start_time < n do
        i += 1
      end

      check_driver_availability(n) if vehicle == "car"
    end
  end

end
