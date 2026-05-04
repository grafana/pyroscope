FROM node:25

WORKDIR /app

COPY package.json yarn.lock .npmrc .
RUN yarn install

COPY index.js .

CMD ["node", "index.js"]
