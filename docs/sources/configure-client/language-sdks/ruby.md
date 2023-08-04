---
title: "Ruby"
menuTitle: "Ruby"
description: "Instrumenting Ruby applications for continuous profiling"
weight: 50
aliases:
  - /docs/phlare/latest/configure-client/language-sdks/ruby
---

# Ruby

## How to add Ruby profiling to your application

Add the `pyroscope` gem to your Gemfile:


```bash
bundle add pyroscope
```

## Pyroscope Ruby gem configuration


Add the following code to your application. If you're using rails, put this into `config/initializers` directory. This code will initialize pyroscope profiler and start profiling:

```ruby
require 'pyroscope'

Pyroscope.configure do |config|
  config.application_name = "my.ruby.app" # replace this with some name for your application
  config.server_address   = "http://my-pyroscope-server:4040" # replace this with the address of your pyroscope server
  # config.auth_token     = "{YOUR_API_KEY}" # optionally, if authentication is enabled, specify the API key
end
```

## How to add profiling labels to Ruby applications

Pyroscope ruby integration provides a number of ways to tag profiling data. For example, you can provide tags when you're initializing the profiler:

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

or you can dynamically tag certain parts of your code:

```ruby
Pyroscope.tag_wrapper({ "controller": "slow_controller_i_want_to_profile" }) do
  slow_code
end
```

## Rails profiling auto-instrumentation

By default, if you add pyroscope to a rails application it will automatically tag your actions with `action="<controller_name>/<action_name>"` tag.

To disable rails autoinstrumentation, set `autoinstrument_rails` to `false`:
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
  # only need tenant_id if multi-tenancy enabled
  # config.tenant_id='<TenantID>'
end
```

To configure Ruby sdk to send data to Pyroscope, replace the `<URL>` placeholder with the appropriate server URL. This could be the Grafana Cloud URL or your own custom Pyroscope server URL.

If you need to send data to Grafana Cloud, you'll have to configure HTTP Basic authentication. Replace `<User>` with your Grafana Cloud stack user and `<Password>` with your Grafana Cloud API key.

If your Pyroscope server has multi-tenancy enabled, you'll need to configure a tenant ID. Replace `<TenantID>` with your Pyroscope tenant ID.

## Ruby profiling examples

Check out the following resources to learn more about Ruby profiling:
- [Ruby examples](https://github.com/pyroscope-io/pyroscope/tree/main/examples/ruby)
- [Ruby Demo](https://demo.pyroscope.io/?query=rideshare-app-ruby.cpu%7B%7D) showing Ruby example with tags
