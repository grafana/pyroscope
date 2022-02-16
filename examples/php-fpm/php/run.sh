#!/bin/bash

php-fpm -D

screen -d -m sh -c "SCRIPT_FILENAME=./index.php REQUEST_METHOD=GET cgi-fcgi -bind -connect 127.0.0.1:9000"

pid=$(pgrep "php-fpm: master process")

pyroscope connect --pid $pid --spy-name phpspy
