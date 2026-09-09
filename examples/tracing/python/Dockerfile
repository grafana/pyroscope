FROM python:3.12

RUN pip3 install flask pyroscope-io==1.2.0 pyroscope-otel==1.0.1
RUN pip3 install opentelemetry-api opentelemetry-sdk opentelemetry-instrumentation-flask opentelemetry-exporter-otlp-proto-grpc

ENV FLASK_ENV=development
ENV PYTHONUNBUFFERED=1

COPY lib ./lib
CMD [ "python", "lib/server.py" ]
