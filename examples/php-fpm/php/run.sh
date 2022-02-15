#!/bin/bash

php-fpm -D

pid=$(pgrep "php-fpm: master process")

pyroscope connect --pid $pid --spy-name phpspy
