import random
import requests
import time

HOSTS = [
    'web',
]

VEHICLES = [
    'bike',
    'scooter',
    'car',
]

if __name__ == "__main__":
    print(f"starting load generator")
    time.sleep(15)
    print('done sleeping')
    while True:
        host = HOSTS[random.randint(0, len(HOSTS) - 1)]
        vehicle = VEHICLES[random.randint(0, len(VEHICLES) - 1)]
        print(f"requesting {vehicle} from {host}")
        try:
            resp = requests.get(f'http://{host}:8000/{vehicle}')
            resp.raise_for_status()
            print(f"received {resp}")
        except BaseException as e:
            print (f"http error {e}")

        time.sleep(random.uniform(0.2, 0.4))
