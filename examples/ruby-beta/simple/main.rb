require "pyroscope_beta"

Pyroscope.configure do |config|
  config.application_name = "test.ruby.app"
  config.server_address = "http://pyroscope:4040"
  config.tags = {
    :region => "us-east-1",
    :hostname => ENV["HOSTNAME"]
  }
end

def work(n)
  i = 0
  while i < n
    i += 1
  end
end

def fast_function
  Pyroscope.tag_wrapper({"function": "fast"}) do
    work(20)
  end
end

def slow_function
  Pyroscope.tag_wrapper({"function": "slow"}) do
    work(80)
  end
end

while true
  fast_function
  slow_function
end
