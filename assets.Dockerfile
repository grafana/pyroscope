#                     _
#                    | |
#   __ _ ___ ___  ___| |_ ___
#  / _` / __/ __|/ _ \ __/ __|
# | (_| \__ \__ \  __/ |_\__ \
#  \__,_|___/___/\___|\__|___/

FROM node:14.17.6-alpine3.12 as js-builder

RUN apk add --no-cache make

WORKDIR /opt/pyroscope

COPY package.json yarn.lock Makefile ./
COPY scripts ./scripts

# we only need the dependencies required to BUILD the application
RUN --mount=type=cache,target=/usr/local/share/.cache/yarn/v6 make install-build-web-dependencies

COPY babel.config.js .eslintrc .eslintignore .prettierrc tsconfig.json ./
COPY webapp ./webapp

ARG EXTRA_METADATA=""

RUN EXTRA_METADATA=$EXTRA_METADATA make assets-release
