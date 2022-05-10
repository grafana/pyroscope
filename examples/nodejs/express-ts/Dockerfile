FROM node:latest

WORKDIR /app

COPY package.json .
RUN yarn 
COPY tsconfig.json .
COPY *.ts .
RUN yarn build
ENV DEBUG=pyroscope
CMD ["yarn", "run", "run"] 

