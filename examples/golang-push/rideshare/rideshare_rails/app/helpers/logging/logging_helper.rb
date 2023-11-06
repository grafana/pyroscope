module Logging
  module LoggingHelper
    def logger_debug(str)
      Rails.logger.info(str)
      i = 0; while i < MULTIPLIER; i += 1; end

      # (MULTIPLIER / 100000).times do
      #   str += Drivers::DriversHelper::BODY[0, 1024]
      # end

      # Rails.logger.debug(str)
    end
  end
end
