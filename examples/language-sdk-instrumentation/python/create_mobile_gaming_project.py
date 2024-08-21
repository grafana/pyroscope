import os
import random
import time

def create_directory_structure(base_path):
    dirs = [
        os.path.join(base_path, "lib"),
        os.path.join(base_path, "lib", "arcade"),
        os.path.join(base_path, "lib", "puzzle"),
        os.path.join(base_path, "lib", "shooter"),
        os.path.join(base_path, "lib", "utility"),
    ]
    for dir_path in dirs:
        os.makedirs(dir_path, exist_ok=True)

def create_files(base_path):
    # Dockerfile
    with open(os.path.join(base_path, "Dockerfile"), "w") as f:
        f.write("""\
FROM python:3.9

RUN pip3 install flask pyroscope-io==0.8.5

ENV FLASK_ENV=development
ENV PYTHONUNBUFFERED=1

COPY lib ./lib
CMD [ "python", "lib/gaming_server.py" ]
""")

    # Dockerfile for load generator
    with open(os.path.join(base_path, "Dockerfile.load-generator"), "w") as f:
        f.write("""\
FROM python:3.9

RUN pip3 install requests

COPY load-generator.py ./load-generator.py

ENV PYTHONUNBUFFERED=1

CMD [ "python", "load-generator.py" ]
""")

    # Docker Compose
    with open(os.path.join(base_path, "docker-compose.yml"), "w") as f:
        f.write("""\
version: '3'
services:
  pyroscope:
    image: grafana/pyroscope
    ports:
      - '4040:4040'

  us-east:
    ports:
      - 5000
    environment:
      - REGION=us-east
    build:
      context: .

  eu-north:
    ports:
      - 5000
    environment:
      - REGION=eu-north
    build:
      context: .

  ap-south:
    ports:
      - 5000
    environment:
      - REGION=ap-south
    build:
      context: .

  load-generator:
    build:
      context: .
      dockerfile: Dockerfile.load-generator
""")

    # Load Generator
    with open(os.path.join(base_path, "load-generator.py"), "w") as f:
        f.write("""\
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
""")

    # Gaming Server
    with open(os.path.join(base_path, "lib", "gaming_server.py"), "w") as f:
        f.write("""\
import os
import pyroscope
from flask import Flask
from arcade.arcade import order_arcade
from puzzle.puzzle import order_puzzle
from shooter.shooter import order_shooter

app_name = os.getenv("PYROSCOPE_APPLICATION_NAME", "flask-gaming-app")
server_address = os.getenv("PYROSCOPE_SERVER_ADDRESS", "http://pyroscope:4040")
basic_auth_username = os.getenv("PYROSCOPE_BASIC_AUTH_USER", "")
basic_auth_password = os.getenv("PYROSCOPE_BASIC_AUTH_PASSWORD", "")
port = int(os.getenv("GAMING_LISTEN_PORT", "5000"))

pyroscope.configure(
    application_name=app_name,
    server_address=server_address,
    basic_auth_username=basic_auth_username,
    basic_auth_password=basic_auth_password,
    tags={"region": f'{os.getenv("REGION")}'}
)

app = Flask(__name__)

@app.route("/arcade")
def arcade():
    order_arcade(0.2)
    return "<p>Arcade game requested</p>"

@app.route("/shooter")
def shooter():
    order_shooter(0.3)
    return "<p>Shooter game requested</p>"

@app.route("/puzzle")
def puzzle():
    order_puzzle(0.4)
    return "<p>Puzzle game requested</p>"

@app.route("/")
def environment():
    result = "<h1>Environment Variables:</h1>"
    for key, value in os.environ.items():
        result += f"<p>{key}={value}</p>"
    return result

if __name__ == "__main__":
    app.run(threaded=False, processes=1, host='0.0.0.0', port=port, debug=False)
""")

    # Arcade Game
    with open(os.path.join(base_path, "lib", "arcade", "arcade.py"), "w") as f:
        f.write("""\
from utility.utility import find_nearest_game, simulate_physics

def order_arcade(search_radius):
    simulate_physics(search_radius, "arcade")
    find_nearest_game(search_radius, "arcade")
""")

    # Puzzle Game
    with open(os.path.join(base_path, "lib", "puzzle", "puzzle.py"), "w") as f:
        f.write("""\
from utility.utility import find_nearest_game, solve_puzzle

def order_puzzle(search_radius):
    solve_puzzle()
    find_nearest_game(search_radius, "puzzle")
""")

    # Shooter Game
    with open(os.path.join(base_path, "lib", "shooter", "shooter.py"), "w") as f:
        f.write("""\
from utility.utility import find_nearest_game, calculate_path

def order_shooter(search_radius):
    calculate_path("enemy", "target")
    find_nearest_game(search_radius, "shooter")
""")

    # Utility Functions
    with open(os.path.join(base_path, "lib", "utility", "utility.py"), "w") as f:
        f.write("""\
import pyroscope
import random
import time

def find_nearest_game(search_radius, game):
    with pyroscope.tag_wrapper({"game": game}):
        for _ in range(int(search_radius * 10**6)):
            _ = random.random() * random.random()

def simulate_physics(time_step, game):
    with pyroscope.tag_wrapper({"game": game, "task": "physics_simulation"}):
        for _ in range(int(time_step * 10**6)):
            _ = random.random() * random.random()

def solve_puzzle():
    with pyroscope.tag_wrapper({"task": "solve_puzzle"}):
        for _ in range(10**7):
            _ = random.random() * random.random()

def calculate_path(start, goal):
    with pyroscope.tag_wrapper({"task": "ai_pathfinding"}):
        for _ in range(10**7):
            _ = random.random() * random.random()
""")

def create_new_project():
    base_path = os.path.join(os.getcwd(), "new_gaming_project")
    os.makedirs(base_path, exist_ok=True)
    
    create_directory_structure(base_path)
    create_files(base_path)
    print(f"New gaming project created at {base_path}")

if __name__ == "__main__":
    create_new_project()
