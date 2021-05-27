FROM lifenz/docker-screenshot:latest

RUN apt-get install -y netcat-traditional
COPY ./take-screenshot.sh /script.sh

ENTRYPOINT ["/bin/sh", "/script.sh"]
