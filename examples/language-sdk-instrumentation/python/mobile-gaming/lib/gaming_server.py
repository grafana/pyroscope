import os
import time
from flask import Flask
from arcade.arcade import order_arcade
from puzzle.puzzle import order_puzzle
from shooter.shooter import order_shooter

port = int(os.getenv("RIDESHARE_LISTEN_PORT", "5000"))

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

