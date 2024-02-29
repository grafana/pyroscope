FROM python:3.9

RUN pip3 install fastapi pyroscope-io==0.8.5 uvicorn[standard]

ENV FLASK_ENV=development
ENV PYTHONUNBUFFERED=1

COPY lib ./lib
CMD [ "uvicorn", "lib.server:app", "--host", "0.0.0.0", "--port", "5000"]

