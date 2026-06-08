FROM python:3.11

COPY requirements.txt .
RUN pip3 install -r requirements.txt

ENV FLASK_ENV=development
ENV PYTHONUNBUFFERED=1

COPY lib ./lib
CMD [ "uvicorn", "lib.server:app", "--host", "0.0.0.0", "--port", "5000"]

