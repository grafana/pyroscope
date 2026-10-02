workers_count = Integer(ENV.fetch("WEB_CONCURRENCY", "4"))

workers workers_count
preload_app! if workers_count.positive?

threads Integer(ENV.fetch("RAILS_MIN_THREADS", "2")), Integer(ENV.fetch("RAILS_MAX_THREADS", "5"))
port Integer(ENV.fetch("PORT", "5000"))
environment ENV.fetch("RAILS_ENV", "production")

# The profiler must be started in the process that serves requests. The sampler
# runs on a background thread and fork(2) does not copy threads, so configuring
# before the preload_app! fork leaves every worker unprofiled. Configuring both
# before and after the fork aborts the worker: the Rust logger is already
# installed in the inherited address space and re-installing it panics.
start_pyroscope = lambda do |role|
  Pyroscope.configure do |config|
    config.application_name = ENV.fetch("PYROSCOPE_APPLICATION_NAME", "ruby-puma-cluster")
    config.server_address = ENV.fetch("PYROSCOPE_SERVER_ADDRESS", "http://pyroscope:4040")
    config.log_level = ENV.fetch("PYROSCOPE_LOG_LEVEL", "info")
    config.report_pid = true
    config.tags = {
      "role" => role,
      "hostname" => ENV.fetch("HOSTNAME", "")
    }
  end

  # preload_app! runs the Rails initializers in the master, where the gem's
  # railtie hook finds no configuration yet and skips controller tagging.
  Pyroscope.initialize_rails_hooks
end

if workers_count.zero?
  after_booted { start_pyroscope.call("single") }
else
  before_worker_boot { |index| start_pyroscope.call("worker-#{index}") }
end
