FROM node:25

WORKDIR /app

COPY package.json yarn.lock .
RUN yarn install
COPY index.js .

ENV DEBUG=pyroscope
ENV PYROSCOPE_WALL_COLLECT_CPU_TIME=true
CMD ["node", "index.js"] 
