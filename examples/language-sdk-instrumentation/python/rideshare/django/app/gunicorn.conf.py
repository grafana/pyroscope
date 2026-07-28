import os

import pyroscope
from django.db import connections


bind = "0.0.0.0:8000"
workers = 2
preload_app = True


def post_fork(server, worker):
    # Don't share connections opened while preloading the app with workers.
    connections.close_all()
    server.log.info("Configuring Pyroscope in worker pid %s", worker.pid)
    pyroscope.configure(
        application_name=os.getenv("PYROSCOPE_APPLICATION_NAME", "ride-sharing-app"),
        server_address=os.getenv("PYROSCOPE_SERVER_ADDRESS", "http://pyroscope:4040"),
        basic_auth_username=os.getenv("PYROSCOPE_BASIC_AUTH_USER", ""),
        basic_auth_password=os.getenv("PYROSCOPE_BASIC_AUTH_PASSWORD", ""),
        tags={
            "region": os.getenv("REGION", ""),
        },
    )
