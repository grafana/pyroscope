FROM python:3

WORKDIR /usr/src/app

COPY --from=pyroscope/pyroscope:latest /usr/bin/pyroscope /usr/bin/pyroscope
COPY main.py ./main.py

ENV PYROSCOPE_APPLICATION_NAME=simple.python.app
ENV PYROSCOPE_SERVER_ADDRESS=http://pyroscope:8080/
CMD ["pyroscope", "exec", "python", "main.py"]
