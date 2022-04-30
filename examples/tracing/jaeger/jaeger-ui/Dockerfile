FROM node:14.17.6-alpine3.12

RUN apk add --no-cache git

WORKDIR /opt/jaeger-ui

# RUN git clone https://github.com/pyroscope-io/jaeger-ui.git /opt/jaeger-ui && git checkout 0b4bdd6a488c0d73265578f1dcb006affb76d4bd
RUN git clone https://github.com/pyroscope-io/jaeger-ui.git /opt/jaeger-ui && git checkout 11733cf166622281ac89e3485ca836a72c689fbd


RUN yarn install || true
ENV HOST=0.0.0.0
RUN sed -i s/localhost/jaeger/ ./packages/jaeger-ui/src/setupProxy.js

ENTRYPOINT [ "/usr/local/bin/yarn" ]

CMD [ "start" ]
