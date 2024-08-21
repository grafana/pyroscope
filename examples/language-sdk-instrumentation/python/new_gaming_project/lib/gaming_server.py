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
