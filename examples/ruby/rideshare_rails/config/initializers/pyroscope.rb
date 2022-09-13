Pyroscope.configure do |config|
  config.app_name       = ENV.fetch("PYROSCOPE_APPLICATION_NAME", "rails-ride-sharing-app")
  config.server_address = ENV.fetch("PYROSCOPE_SERVER_ADDRESS", "http://pyroscope:4040")
  config.auth_token     = ENV.fetch("PYROSCOPE_AUTH_TOKEN", "")

  config.tags = {
    "region": ENV["REGION"] || "us-east",
  }
end
