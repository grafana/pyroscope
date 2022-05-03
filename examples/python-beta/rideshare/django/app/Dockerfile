# pull official base image
FROM python:3.9

USER root

# install rust toolchain
RUN curl https://sh.rustup.rs -sSf | sh -s -- --default-toolchain stable -y 
ENV PATH=$PATH:/root/.cargo/bin
# install libunwind, docker requires switching to root user
RUN apt-get update 
RUN apt-get install -y libunwind8-dev libssl-dev

# set work directory
WORKDIR /usr/src/app

# set environment variables
ENV PYTHONDONTWRITEBYTECODE 1
ENV PYTHONUNBUFFERED 1

# install psycopg2 dependencies
# RUN apk update \
#     && apk add libffi-dev postgresql-dev gcc python3-dev musl-dev

RUN apt-get update -y
RUN apt-get install -y postgresql postgresql-client netcat gcc python3-dev musl-dev


# install dependencies
RUN pip install --upgrade pip
COPY ./requirements.txt .
RUN pip install -r requirements.txt

# copy entrypoint.sh
COPY ./entrypoint.sh .
RUN sed -i 's/\r$//g' /usr/src/app/entrypoint.sh
RUN chmod +x /usr/src/app/entrypoint.sh

# copy project
COPY . .

# run entrypoint.sh
ENTRYPOINT ["/usr/src/app/entrypoint.sh"]
