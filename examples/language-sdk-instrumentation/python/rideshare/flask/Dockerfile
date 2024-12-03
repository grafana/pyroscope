FROM python:3.9

RUN pip3 install flask pyroscope-io==0.8.8 pyroscope-otel==0.4.0
RUN pip3 install opentelemetry-api opentelemetry-sdk opentelemetry-instrumentation-flask opentelemetry-exporter-otlp-proto-grpc

ENV FLASK_ENV=development
ENV PYTHONUNBUFFERED=1

COPY lib ./lib
CMD [ "python", "lib/server.py" ]
