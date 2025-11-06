FROM node:23

WORKDIR /app

COPY package.json yarn.lock .
RUN yarn

COPY tsconfig.json .
COPY *.ts .
RUN yarn build

ENV DEBUG=pyroscope
CMD ["yarn", "start"]
