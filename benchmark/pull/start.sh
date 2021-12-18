#!/bin/sh

cat server.yml.example | grep -B 1000 'static-configs:' > server.yml

for ((x=0 ; x<$1 ; x++)); do
  echo "      - application: pull-target" >> server.yml
  echo "        targets:"                 >> server.yml
  echo "          - pull-target:4042"     >> server.yml
  echo "        labels:"                  >> server.yml
  echo "          pod: pod-$x"            >> server.yml
done

docker-compose up --build
