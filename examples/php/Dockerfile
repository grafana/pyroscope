FROM php:7.3.27

WORKDIR /var/www/html

COPY --from=pyroscope/pyroscope:dev /usr/bin/pyroscope /usr/bin/pyroscope
COPY main.php ./main.php

ENV PYROSCOPE_APPLICATION_NAME=simple.php.app
ENV PYROSCOPE_SERVER_ADDRESS=http://pyroscope:4040/
ENV PYROSCOPE_LOG_LEVEL=debug
CMD ["pyroscope", "exec", "php", "main.php"]
