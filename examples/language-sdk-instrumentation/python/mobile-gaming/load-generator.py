import random
import requests
import time

HOSTS = [
    'us-east',
    'eu-north',
    'ap-south',
]

VEHICLES = [
    'arcade',
    'shooter',
    'puzzle',
]

if __name__ == "__main__":
    print(f"starting load generator")
    time.sleep(3)
    while True:
        host = HOSTS[random.randint(0, len(HOSTS) - 1)]
        game = VEHICLES[random.randint(0, len(VEHICLES) - 1)]
        print(f"requesting {game} from {host}")
        try:
            resp = requests.get(f'http://{host}:5000/{game}')
            resp.raise_for_status()
            print(f"received {resp}")
        except BaseException as e:
            print (f"http error {e}")

        time.sleep(random.uniform(0.2, 0.4))
