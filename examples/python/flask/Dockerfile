FROM python:3.9

# RUN pip3 install pipenv
# COPY Pipfile ./Pipfile
# COPY Pipfile.lock ./Pipfile.lock
# RUN pipenv install

RUN pip3 install flask pyroscope-io==0.4.0

ENV FLASK_ENV=development
ENV PYTHONUNBUFFERED=1

COPY lib ./lib
CMD [ "python", "lib/server.py" ]

