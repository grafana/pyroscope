import time
# import pyroscope
import os
from datetime import datetime


def mutex_lock(n):
    i = 0
    start_time = time.time()
    while time.time() - start_time < n * 10:
        i += 1

def check_driver_availability(n):
    i = 0
    start_time = time.time()
    while time.time() - start_time < n / 2:
        i += 1

    # Every 4 minutes this will artificially create make requests in eu-north region slow
    # this is just for demonstration purposes to show how performance impacts show up in the
    # flamegraph

    force_mutex_lock = datetime.today().minute * 4 % 8 == 0
    if os.getenv("REGION") == "eu-north" and force_mutex_lock:
        mutex_lock(n)

# Add a tag to that puts the "game" variable into an `operation` label
def find_nearest_game(n, game):
    i = 0
    start_time = time.time()
    while time.time() - start_time < n:
        i += 1
    if game == "puzzle":
        check_driver_availability(n)
