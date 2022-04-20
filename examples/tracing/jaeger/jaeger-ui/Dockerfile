FROM node:14.17.6-alpine3.12

RUN apk add --no-cache git

WORKDIR /opt/jaeger-ui

RUN git clone https://github.com/pyroscope-io/jaeger-ui.git /opt/jaeger-ui && git checkout c32127fdb7cf8a4c88541ebc7bd3703437d2f121

RUN yarn install || true
ENV HOST=0.0.0.0
RUN sed -i s/localhost/jaeger/ ./packages/jaeger-ui/src/setupProxy.js

ENTRYPOINT [ "/usr/local/bin/yarn" ]

CMD [ "start" ]
