module Drivers
  module DriversHelper
    extend Logging::LoggingHelper
    BODY = (SecureRandom.hex(1024) * 1000000)[0,MULTIPLIER*800]


    def prepare_drivers_response
      logger_debug("prepare_drivers_response")
      i = 0; t = rand(MULTIPLIER * 1.2); while i < t; i += 1; end
      generate_drivers_data
    end

    def generate_drivers_data
      logger_debug("generate_drivers_data")
      i = 0; t = rand(MULTIPLIER * 1.0); while i < t; i += 1; end
      prepare_response_body
    end

    def prepare_response_body
      logger_debug("prepare_response_body")
      i = 0; t = rand(MULTIPLIER * 0.8); while i < t; i += 1; end
      compress_response
    end

    def compress_response
      logger_debug("compress_response")
      i = 0; t = rand(MULTIPLIER * 1.0); while i < t; i += 1; end
      quality = ENV["COMPRESSION"] == "low" ? 2 : 11
      i=0
      while i < 4
        Brotli.deflate(BODY, :quality => quality)
        i+=1
      end
    end
  end
end
