#!/usr/bin/env ruby

# go run ./cmd loadgen


values = 8
loop do
  values = values * 2
  env_vars = <<-ENV
    PYROBENCH_APPS=1
    PYROBENCH_CLIENTS=1
    PYROBENCH_TAG_KEYS=1
    PYROBENCH_TAG_VALUES=#{values}
    PYROBENCH_REQUESTS=#{[values, 10000].max}
  ENV
  env_vars = env_vars.lines.map { |x| x.strip + "\n" }.join("")
  File.write('./run-parameters.env', env_vars)

  puts "values: #{values}"
  system "sh start.sh"
  # system "time curl 'http://localhost:4040/render?from=now-7d&until=now&query=2ac732e9f8fcf65d17b0b9962894c8f13d5fda1fec54b3e210%7B%7D&refreshToken=0.8350450435879586&max-nodes=1024&format=json' > /dev/null"
end
