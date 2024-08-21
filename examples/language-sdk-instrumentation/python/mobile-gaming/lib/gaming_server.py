import os
import time
import pyroscope
from flask import Flask
from arcade.arcade import order_arcade
from puzzle.puzzle import order_puzzle
from shooter.shooter import order_shooter

app_name = os.getenv("PYROSCOPE_APPLICATION_NAME", "flask-arcade-app")
gaming_server_addr = os.getenv("PYROSCOPE_SERVER_ADDRESS", "http://pyroscope:4040")
basic_auth_username = os.getenv("PYROSCOPE_BASIC_AUTH_USER", "")
basic_auth_password = os.getenv("PYROSCOPE_BASIC_AUTH_PASSWORD", "")
port = int(os.getenv("RIDESHARE_LISTEN_PORT", "5000"))

pyroscope.configure(
	application_name = app_name,
	server_address   = gaming_server_addr,
    basic_auth_username = basic_auth_username, # for grafana cloud
    basic_auth_password = basic_auth_password, # for grafana cloud
	tags             = {
        "region":   f'{os.getenv("REGION")}',
	}
)

app = Flask(__name__)

@app.route("/arcade")
def arcade():
    order_arcade(0.2)
    return "<p>Arcade ordered</p>"


@app.route("/shooter")
def shooter():
    order_shooter(0.3)
    return "<p>Shooter ordered</p>"


@app.route("/puzzle")
def puzzle():
    order_puzzle(0.4)
    return "<p>Puzzle ordered</p>"


@app.route("/")
def environment():
    result = "<h1>environment vars:</h1>"
    for key, value in os.environ.items():
        result +=f"<p>{key}={value}</p>"
    return result

if __name__ == '__main__':
    app.run(threaded=False, processes=1, host='0.0.0.0', port=port, debug=False)

