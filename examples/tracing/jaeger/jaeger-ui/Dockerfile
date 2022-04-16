FROM node:14.17.6-alpine3.12

RUN apk add --no-cache git

WORKDIR /opt/jaeger-ui

RUN git clone https://github.com/pyroscope-io/jaeger-ui.git /opt/jaeger-ui
RUN git checkout 5fe059aafd9f7c6c2c249d49bd5ad4eecb7ef881

RUN yarn install || true
ENV HOST=0.0.0.0
RUN sed -i s/localhost/jaeger/ ./packages/jaeger-ui/src/setupProxy.js

ENTRYPOINT [ "/usr/local/bin/yarn" ]

CMD [ "start" ]
