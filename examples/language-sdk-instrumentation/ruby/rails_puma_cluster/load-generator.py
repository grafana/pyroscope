import random
import requests
import time

HOSTS = [
    'cluster',
    'single',
]

ENDPOINTS = [
    'fast',
    'slow',
]

if __name__ == "__main__":
    print(f"starting load generator")
    time.sleep(3)
    while True:
        host = HOSTS[random.randint(0, len(HOSTS) - 1)]
        endpoint = ENDPOINTS[random.randint(0, len(ENDPOINTS) - 1)]
        print(f"requesting {endpoint} from {host}")
        try:
            resp = requests.get(f'http://{host}:5000/{endpoint}')
            resp.raise_for_status()
            print(f"received {resp}")
        except BaseException as e:
            print (f"http error {e}")

        time.sleep(random.uniform(0.05, 0.15))
