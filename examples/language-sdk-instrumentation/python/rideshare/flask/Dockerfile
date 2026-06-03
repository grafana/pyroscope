FROM python:3.12-slim

COPY requirements.txt .
RUN pip3 install -r requirements.txt

ENV FLASK_ENV=development
ENV PYTHONUNBUFFERED=1

COPY lib ./lib
CMD [ "python", "lib/server.py" ]
