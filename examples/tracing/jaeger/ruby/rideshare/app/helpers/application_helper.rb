module ApplicationHelper
  class << self
    def find_nearest_vehicle(radius, vehicle)
      from = Time.now.to_f
      to = from + radius * 0.2
      while Time.now.to_f < to

      end
    end
  end
end
