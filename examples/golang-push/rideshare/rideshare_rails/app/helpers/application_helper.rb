
require "pyroscope"

module ApplicationHelper
  extend Drivers::DriversHelper
  extend Strings::StringsHelper
  extend Logging::LoggingHelper
  extend Mutexes::MutexesHelper

  def check_driver_availability(n)
    current_minute = Time.new.strftime('%M').to_i
    force_mutex_lock = (current_minute % 2) == 0

    i = 0; while i < MULTIPLIER * 5; i += 1; end
    mutex_lock(n) if ENV["REGION"] == "eu-north" and force_mutex_lock
  end

  def find_nearest_vehicle(n, vehicle)
    logger_debug("find_nearest_vehicle")
    i = 0; while i < MULTIPLIER * 0.25; i += 1; end
    traverse_vehicle_options(n, vehicle)
  end

  def traverse_vehicle_options(n, vehicle)
    logger_debug("traverse_vehicle_options")
    i = 0; while i < MULTIPLIER * 0.33; i += 1; end
    compile_options(n, vehicle)
  end

  def compile_options(n, vehicle)
    logger_debug("compile_options")
    i = 0; while i < MULTIPLIER * 0.2; i += 1; end
    build_list_of_options
    if vehicle == "car"
      check_driver_availability(n)
      prepare_drivers_response
    end
  end
end
