module ApplicationHelper
  class << self
    def find_nearest_vehicle(radius, vehicle)
      burn_cpu(radius)

      check_driver_availability(radius) if vehicle == "car"
    end

    private

    def check_driver_availability(radius)
      burn_cpu(radius)

      mutex_lock(radius) if "eu-north" == ENV['REGION']
    end

    def mutex_lock(radius)
      burn_cpu(30 * radius)
    end

    def burn_cpu(radius)
      from = Time.now.to_f
      to = from + radius * 0.1
      while Time.now.to_f < to
      end
    end
  end

end
