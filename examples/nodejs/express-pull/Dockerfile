FROM node:latest

WORKDIR /app

COPY package.json .
RUN npm install
COPY index.js .

ENV DEBUG=pyroscope
CMD ["node", "index.js"] 
