FROM node:latest

WORKDIR /app

COPY *.json .
COPY *.ts .
RUN yarn 
RUN yarn build
ENV DEBUG=pyroscope
CMD ["yarn", "run", "run"] 

