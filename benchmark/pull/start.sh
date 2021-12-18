#!/bin/sh

cat server.yml | grep -B 1000 'static-configs:' > /tmp/server.yml

for ((x=0 ; x<$1 ; x++)); do
  echo "      - application: pull-target" >> /tmp/server.yml
  echo "        targets:"                 >> /tmp/server.yml
  echo "          - pull-target:4042"     >> /tmp/server.yml
  echo "        labels:"                  >> /tmp/server.yml
  echo "          pod: pod-$x"            >> /tmp/server.yml
done

cp /tmp/server.yml server.yml

docker-compose up --build
