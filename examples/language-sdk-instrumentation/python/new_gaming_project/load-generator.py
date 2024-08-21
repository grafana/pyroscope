import random
import requests
import time

HOSTS = [
    'us-east',
    'eu-north',
    'ap-south',
]

GAMES = [
    'arcade',
    'shooter',
    'puzzle',
]

if __name__ == "__main__":
    print(f"starting load generator")
    time.sleep(3)
    while True:
        host = HOSTS[random.randint(0, len(HOSTS) - 1)]
        game = GAMES[random.randint(0, len(GAMES) - 1)]
        print(f"requesting {game} from {host}")
        try:
            resp = requests.get(f'http://{host}:5000/{game}')
            resp.raise_for_status()
            print(f"received {resp}")
        except Exception as e:
            print (f"http error {e}")

        # Simulate some CPU work
        for _ in range(10**7):
            _ = random.random() * random.random()

        time.sleep(random.uniform(0.2, 0.4))
