FROM node:latest

WORKDIR /app

COPY package.json yarn.lock .
COPY tsconfig.json .
RUN yarn 
COPY *.ts .
RUN yarn build
ENV DEBUG=pyroscope
CMD ["yarn", "run", "run"] 

