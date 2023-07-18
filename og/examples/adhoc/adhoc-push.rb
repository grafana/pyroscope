#!/usr/bin/env ruby
require "pyroscope"

def is_prime(n)
  i = 2
  while i < n do
    return true if i * i > n
    return false if n % i == 0
    i += 1
  end
  false
end

def slow(n)
  (1..n).reduce(:+)
end

def fast(n)
  root = Math::sqrt(n).to_i
  (1..n).step(root).map{|a|
    b = [a + root - 1, n].min
    (b - a + 1) * (a + b) / 2
  }.reduce(:+)
end

def run
  base = rand(1..10**6)
  (0..2*10**6).each do |i|
    is_prime(base + i) ? slow(rand(1..10**4)) : fast(rand(1..10**4))
  end
end

Pyroscope.configure do |config|
  # No need to modify existing settings,
  # pyroscope will override the server address
  config.app_name = "adhoc.push.ruby"
  config.server_address = "http://pyroscope:4040"
end
run
