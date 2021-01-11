def work(n)
  i = 0
  while i < n
    i += 1
  end
end

def fast_function
  work(20000)
end

def slow_function
  work(80000)
end

while true
  fast_function
  slow_function
end
