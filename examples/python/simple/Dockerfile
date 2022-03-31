FROM python:3.9

WORKDIR /usr/src/app

RUN adduser --disabled-password --gecos --quiet pyroscope
USER pyroscope

COPY --from=pyroscope/pyroscope:latest /usr/bin/pyroscope /usr/bin/pyroscope
COPY main.py ./main.py
COPY requirements.txt ./requirements.txt

ENV PYROSCOPE_APPLICATION_NAME=simple.python.app
ENV PYROSCOPE_SERVER_ADDRESS=http://pyroscope:4040/
ENV PYROSCOPE_LOG_LEVEL=debug

RUN pip install -r requirements.txt

CMD ["python", "main.py"]
