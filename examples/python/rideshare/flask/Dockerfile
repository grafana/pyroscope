FROM python:3.9

RUN pip3 install flask pyroscope-io==0.8.5

ENV FLASK_ENV=development
ENV PYTHONUNBUFFERED=1

COPY lib ./lib
CMD [ "python", "lib/server.py" ]

