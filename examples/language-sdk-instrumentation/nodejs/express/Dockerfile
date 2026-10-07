# Must stay glibc: the SDK ships prebuilt binaries.
FROM node:25-trixie-slim

WORKDIR /app

COPY package.json yarn.lock .npmrc .yarnrc .
RUN yarn install
COPY index.js .

ENV DEBUG=pyroscope
ENV PYROSCOPE_WALL_COLLECT_CPU_TIME=true
CMD ["node", "index.js"]
