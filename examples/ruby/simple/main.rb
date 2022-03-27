require "pyroscope"

Pyroscope.configure do |config|
  config.app_name = "test.ruby.app"
  config.server_address = "http://pyroscope:4040/"
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
  Pyroscope.tag_wrapper({ "function" => "fast" }) do
    work(20000)
  end
end

def slow_function
  Pyroscope.tag({ "function" => "slow" })
    work(80000)
  Pyroscope.remove_tags("function")
end

while true
  fast_function
  slow_function
end
