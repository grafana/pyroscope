FROM php:7.3.27

WORKDIR /var/www/html

COPY --from=pyroscope/pyroscope:latest /usr/bin/pyroscope /usr/bin/pyroscope
COPY main.php ./main.php

ENV PYROSCOPE_APPLICATION_NAME=simple.php.app
ENV PYROSCOPE_SERVER_ADDRESS=http://pyroscope:4040/
ENV PYROSCOPE_LOG_LEVEL=debug

RUN adduser --disabled-password --gecos --quiet pyroscope
USER pyroscope

CMD ["pyroscope", "exec", "php", "main.php"]
