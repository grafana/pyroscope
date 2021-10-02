import time
import pyroscope_io as pyroscope

def find_nearest_vehicle(n, vehicle):
    pyroscope.tag({ "vehicle": vehicle})
    i = 0
    start_time = time.time()
    while time.time() - start_time < n:
        i += 1
    pyroscope.remove_tags("vehicle")
