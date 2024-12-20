import os

import pyroscope
from flask import Flask

# OpenTelemetry
from opentelemetry.instrumentation.flask import FlaskInstrumentor
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from pyroscope.otel import PyroscopeSpanProcessor

from bike.bike import order_bike
from car.car import order_car
from scooter.scooter import order_scooter

provider = TracerProvider()
provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter()))
provider.add_span_processor(PyroscopeSpanProcessor())

# Sets the global default tracer provider
trace.set_tracer_provider(provider)

app_name = os.getenv("PYROSCOPE_APPLICATION_NAME", "rideshare.python.push.app")
server_addr = os.getenv("PYROSCOPE_SERVER_ADDRESS", "http://pyroscope:4040")
basic_auth_username = os.getenv("PYROSCOPE_BASIC_AUTH_USER", "")
basic_auth_password = os.getenv("PYROSCOPE_BASIC_AUTH_PASSWORD", "")
port = int(os.getenv("RIDESHARE_LISTEN_PORT", "5000"))

pyroscope.configure(
	application_name = app_name,
	server_address   = server_addr,
    basic_auth_username = basic_auth_username, # for grafana cloud
    basic_auth_password = basic_auth_password, # for grafana cloud
	tags             = {
        "region":   f'{os.getenv("REGION")}',
	}
)

app = Flask(__name__)
FlaskInstrumentor().instrument_app(app)

@app.route("/bike")
def bike():
    order_bike(0.2)
    return "<p>Bike ordered</p>"

@app.route("/scooter")
def scooter():
    order_scooter(0.3)
    return "<p>Scooter ordered</p>"

@app.route("/car")
def car():
    order_car(0.4)
    return "<p>Car ordered</p>"


@app.route("/")
def environment():
    result = "<h1>environment vars:</h1>"
    for key, value in os.environ.items():
        result +=f"<p>{key}={value}</p>"
    return result

if __name__ == '__main__':
    app.run(threaded=False, processes=1, host='0.0.0.0', port=port, debug=False)
