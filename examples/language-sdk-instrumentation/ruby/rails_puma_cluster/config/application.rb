require_relative "boot"

require "rails"
require "action_controller/railtie"

Bundler.require(*Rails.groups)

module RailsPumaCluster
  class Application < Rails::Application
    config.load_defaults 7.2
    config.api_only = true
    config.eager_load = true
    config.secret_key_base = "example-only-not-for-production"
    config.logger = ActiveSupport::Logger.new($stdout)
    config.log_level = :info
  end
end
