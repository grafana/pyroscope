#                     _
#                    | |
#   __ _ ___ ___  ___| |_ ___
#  / _` / __/ __|/ _ \ __/ __|
# | (_| \__ \__ \  __/ |_\__ \
#  \__,_|___/___/\___|\__|___/

FROM node:14.17.6-alpine3.12 as js-builder

RUN apk update && apk upgrade && \
    apk add --no-cache make

WORKDIR /opt/pyroscope

COPY scripts ./scripts
COPY package.json yarn.lock Makefile lerna.json ./
COPY lib ./lib
COPY packages ./packages
COPY babel.config.js .eslintrc.js .eslintignore .prettierrc tsconfig.json ./
COPY webapp ./webapp

# we only need the dependencies required to BUILD the application
RUN make install-build-web-dependencies


ARG EXTRA_METADATA=""

RUN EXTRA_METADATA=$EXTRA_METADATA make assets-release
