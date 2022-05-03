FROM python:3.9

# RUN pip3 install pipenv
# COPY Pipfile ./Pipfile
# COPY Pipfile.lock ./Pipfile.lock
# RUN pipenv install

USER root

# install rust toolchain
RUN curl https://sh.rustup.rs -sSf | sh -s -- --default-toolchain stable -y 
ENV PATH=$PATH:/root/.cargo/bin
# install libunwind, docker requires switching to root user
RUN apt-get update 
RUN apt-get install -y libunwind8-dev libssl-dev

RUN pip3 install fastapi pyroscope-beta uvicorn[standard]

ENV FLASK_ENV=development
ENV PYTHONUNBUFFERED=1

COPY lib ./lib
CMD [ "uvicorn", "lib.server:app", "--host", "0.0.0.0", "--port", "5000"]

