module Strings
  module StringsHelper
    extend Logging::LoggingHelper
    def build_list_of_options
      str = ""
      i = 0
      while i < MULTIPLIER / 1000
        str += Drivers::DriversHelper::BODY[0, 1024]
        i += 1
      end
    end
  end
end
