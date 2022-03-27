#!/usr/bin/env python3

import os

import pyroscope

pyroscope.configure(
	app_name       = "simple.python.app",
	server_address = "http://pyroscope:4040",
	tags           = {
    "hostname": os.getenv("HOSTNAME"),
	}
)

def work(n):
	i = 0
	while i < n:
		i += 1

def fast_function():
	with pyroscope.tag_wrapper({ "function": "fast" }):
		work(20000)

def slow_function():
	pyroscope.tag({ "function": "slow" })
	work(80000)
	pyroscope.remove_tags("function")

if __name__ == "__main__":
	while True:
		fast_function()
		slow_function()
