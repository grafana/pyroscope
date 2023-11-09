import random
import requests
import time

HOSTS = [
    'us-east',
    'eu-north',
    'ap-south',
    'ap-south-alpine',
]

VEHICLES = [
    'bike',
    'scooter',
    'car',
]

if __name__ == "__main__":
    print(f"starting load generator")
    time.sleep(3)
    while True:
        host = HOSTS[random.randint(0, len(HOSTS) - 1)]
        vehicle = VEHICLES[random.randint(0, len(VEHICLES) - 1)]
        print(f"requesting {vehicle} from {host}")
        resp = requests.get(f'http://{host}:5000/{vehicle}')
        print(f"received {resp}")
        time.sleep(random.uniform(0.2, 0.4))
