require "pyroscope"

Pyroscope.configure do |config|
  config.app_name = "test.ruby.app{}"
  config.server_address = "http://pyroscope:4040/"
end

def work(n)
  i = 0
  while i < n
    i += 1
  end
end

def fast_function
  Pyroscope.set_tag("function", "fast")
  work(20000)
  Pyroscope.set_tag("function", "")
end

def slow_function
  Pyroscope.set_tag("function", "slow")
  work(80000)
  Pyroscope.set_tag("function", "")
end

while true
  fast_function
  slow_function
end
