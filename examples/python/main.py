#!/usr/bin/env python3

import pyroscope_io

pyroscope_io.configure(pyroscope_io.Config("simple.python.app", "http://pyroscope:4040"))
pyroscope_io.start()

def work(n):
	i = 0
	while i < n:
		i += 1

def fast_function():
	pyroscope_io.set_tag("function", "fast")
	work(20000)
	pyroscope_io.set_tag("function", "")

def slow_function():
	pyroscope_io.set_tag("function", "slow")
	work(80000)
	pyroscope_io.set_tag("function", "")

if __name__ == "__main__":
	while True:
		fast_function()
		slow_function()
