---
title: "Ruby"
menuTitle: "Ruby"
description: "Instrumenting Ruby applications for continuous profiling."
weight: 50
aliases:
  - /docs/phlare/latest/configure-client/language-sdks/ruby
---

# Ruby

The Ruby Profiler revolutionizes performance tuning in Ruby applications. Integrated with Pyroscope, it offers real-time performance data, allowing developers to delve deep into their Ruby codebase. This tool is essential for identifying performance issues, optimizing code efficiency, and enhancing the overall speed and reliability of Ruby applications.

## Before you begin

To capture and analyze profiling data, you need either a hosted Pyroscope OSS server or a hosted [Pryoscope instance with Grafana Cloud Profiles](/products/cloud/profiles-for-continuous-profiling/) (requires a free Grafana Cloud account).

The Pyroscope server can be a local server for development or a remote server for production use.

## Add Ruby profiling to your application

Add the `pyroscope` gem to your Gemfile:

```bash
bundle add pyroscope
```

## Configure the Ruby client

Add the following code to your application. If you're using Rails, put this into `config/initializers` directory. This code will initialize the Pyroscope profiler and start profiling:

```ruby
require 'pyroscope'

Pyroscope.configure do |config|
  config.application_name = "my.ruby.app" # replace this with some name for your application
  config.server_address   = "http://my-pyroscope-server:4040" # replace this with the address of your Pyroscope server
end
```

## How to add profiling labels to Ruby applications

The Pyroscope Ruby integration provides a number of ways to tag profiling data. For example, you can provide tags when you're initializing the profiler:

```ruby
require 'pyroscope'

Pyroscope.configure do |config|
  config.application_name = "my.ruby.app"
  config.server_address   = "http://my-pyroscope-server:4040"

  config.tags = {
    "hostname" => ENV["HOSTNAME"],
  }
end
```

Or you can dynamically tag certain parts of your code:

```ruby
Pyroscope.tag_wrapper({ "controller": "slow_controller_i_want_to_profile" }) do
  slow_code
end
```

## Rails profiling auto-instrumentation

By default, if you add Pyroscope to a Rails application it will automatically tag your actions with a `action="<controller_name>/<action_name>"` tag.

To disable Rails auto-instrumentation, set `autoinstrument_rails` to `false`:
```ruby
Pyroscope.configure do |config|
  config.autoinstrument_rails = false
  # more configuration
end
```

## Sending data to Pyroscope OSS or Grafana Cloud Profiles with Ruby SDK

```ruby
require "pyroscope"

Pyroscope.configure do |config|
  config.application_name = "example.ruby.app"
  config.server_address = "<URL>"
  config.basic_auth_username='<User>'
  config.basic_auth_password='<Password>'
  # Optional Pyroscope tenant ID (only needed if using multi-tenancy). Not needed for Grafana Cloud.
  # config.tenant_id='<TenantID>'
end
```

To configure the Ruby SDK to send data to Pyroscope, replace the `<URL>` placeholder with the appropriate server URL. This could be the Grafana Cloud URL or your own custom Pyroscope server URL.

If you need to send data to Grafana Cloud, you'll have to configure HTTP Basic authentication. Replace `<User>` with your Grafana Cloud stack user and `<Password>` with your Grafana Cloud API key.

If your Pyroscope server has multi-tenancy enabled, you'll need to configure a tenant ID. Replace `<TenantID>` with your Pyroscope tenant ID.

## Ruby profiling examples

Check out the following resources to learn more about Ruby profiling:
- [Ruby examples](https://github.com/pyroscope-io/pyroscope/tree/main/examples/language-sdk-instrumentation/ruby)
- [Ruby Demo](https://demo.pyroscope.io/?query=rideshare-app-ruby.cpu%7B%7D) showing Ruby example with tags
