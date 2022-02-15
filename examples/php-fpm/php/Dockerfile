FROM php:8.1-fpm-alpine

RUN apk add binutils

WORKDIR /var/www/html

COPY --from=pyroscope/pyroscope:latest /usr/bin/pyroscope /usr/bin/pyroscope
COPY php/run.sh /tmp
COPY php/index.php ./index.php

ENV PYROSCOPE_APPLICATION_NAME=simple.php.app
ENV PYROSCOPE_SERVER_ADDRESS=http://pyroscope:4040/
ENV PYROSCOPE_LOG_LEVEL=debug

CMD ["sh", "/tmp/run.sh"]
