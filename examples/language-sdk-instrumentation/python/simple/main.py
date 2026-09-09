#!/usr/bin/env python3

import logging
import os
import pyroscope

l = logging.getLogger()
l.setLevel(logging.DEBUG)

ALLOCATION_SIZE = 64 * 1024
MAX_RETAINED_ALLOCATIONS = 256
RETAINED_ALLOCATIONS_AFTER_TRIM = 128
retained_allocations = []

addr = os.getenv("PYROSCOPE_SERVER_ADDRESS") or "http://pyroscope:4040"
print(addr)

pyroscope.configure(
	application_name = "simple.python.app",
	server_address = addr,
	enable_logging = True,
	mem_enabled = True,
)

def work(n):
	i = 0
	while i < n:
		i += 1

def allocate_memory(chunks):
	for _ in range(chunks):
		retained_allocations.append(bytearray(ALLOCATION_SIZE))
	if len(retained_allocations) >= MAX_RETAINED_ALLOCATIONS:
		del retained_allocations[:-RETAINED_ALLOCATIONS_AFTER_TRIM]

def fast_function():
	with pyroscope.tag_wrapper({ "function": "fast" }):
		work(20000)
		allocate_memory(1)

def slow_function():
	with pyroscope.tag_wrapper({ "function": "slow" }):
	    work(80000)
	    allocate_memory(4)

if __name__ == "__main__":
	while True:
		fast_function()
		slow_function()
