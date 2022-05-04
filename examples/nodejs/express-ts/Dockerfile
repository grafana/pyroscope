FROM node:latest

WORKDIR /app

COPY *.json .
RUN yarn 
COPY *.ts .
COPY profiler.js .

RUN yarn 
