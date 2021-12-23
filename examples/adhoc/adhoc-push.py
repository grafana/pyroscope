#!/usr/bin/env python
from random import randint
from math import sqrt

import pyroscope


def is_prime(n):
    for i in range(2, n):
        if i * i > n:
            return True
        if n % i == 0:
            return False
    return False


def slow(n):
    sum = 0
    for i in range(1, n+1):
        sum += i
    return sum


def fast(n):
    sum = 0
    root = int(sqrt(n))
    for a in range(1, n+1, root):
        b = min(a + root - 1, n)
        sum += (b - a + 1) * (a + b) // 2
    return sum


def run():
    base = randint(1, 10**6)
    for i in range(2*10**6):
        [fast, slow][is_prime(base + i)](randint(1, 10**4))


if __name__ == '__main__':
    # No need to modify existing settings,
    # pyroscope will override the server address
    pyroscope.configure(
        app_name="adhoc.push.python",
        server_address="http://pyroscope:4040",
    )
    run()
