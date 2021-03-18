#!/usr/bin/env python3

def work(n):
	i = 0
	while i < n:
		i += 1


def fast_function():
	work(20000)


def slow_function():
	work(80000)


if __name__ == "__main__":
	while True:
		fast_function()
		slow_function()
