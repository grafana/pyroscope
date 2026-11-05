import time
import pyroscope
import os
from datetime import datetime

ALLOCATION_SIZE = 64 * 1024
MAX_RETAINED_ALLOCATIONS = 256
RETAINED_ALLOCATIONS_AFTER_TRIM = 128
ALLOCATION_CHUNKS_BY_VEHICLE = {
    "bike": 1,
    "scooter": 2,
    "car": 4,
}
retained_allocations = []


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


def allocate_vehicle_memory(vehicle):
    for _ in range(ALLOCATION_CHUNKS_BY_VEHICLE[vehicle]):
        retained_allocations.append(bytearray(ALLOCATION_SIZE))
    if len(retained_allocations) >= MAX_RETAINED_ALLOCATIONS:
        del retained_allocations[:-RETAINED_ALLOCATIONS_AFTER_TRIM]


def find_nearest_vehicle(n, vehicle):
    with pyroscope.tag_wrapper({ "vehicle": vehicle}):
        i = 0
        start_time = time.time()
        while time.time() - start_time < n:
            i += 1
        allocate_vehicle_memory(vehicle)
        if vehicle == "car":
            check_driver_availability(n)
