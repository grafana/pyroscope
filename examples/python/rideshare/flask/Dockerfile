ARG PYTHON_VERSION=3.9
FROM python:${PYTHON_VERSION}

RUN pip3 install flask pyroscope-io==0.8.5

ENV FLASK_ENV=development
ENV PYTHONUNBUFFERED=1

COPY lib ./lib
CMD [ "python", "lib/server.py" ]

