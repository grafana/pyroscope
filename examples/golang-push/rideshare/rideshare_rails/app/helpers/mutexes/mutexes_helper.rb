module Mutexes
  module MutexesHelper
    extend Logging::LoggingHelper
    def mutex_lock(n)
      logger_debug("mutex lock")
      i = 0; while i < MULTIPLIER * 5 * n*5; i += 1; end
    end
  end
end
