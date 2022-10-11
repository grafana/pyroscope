FROM node:16-alpine3.15 as js-builder

ENV NODE_OPTIONS=--max_old_space_size=8000

WORKDIR /grafana

COPY grafana/phlare-datasource/package.json grafana/phlare-datasource/README.md grafana/phlare-datasource/.prettierrc.js grafana/phlare-datasource/tsconfig.json grafana/phlare-datasource/yarn.lock  phlare-datasource/
RUN cd phlare-datasource && yarn install

COPY grafana/flamegraph/package.json grafana/flamegraph/README.md  grafana/flamegraph/.prettierrc.js grafana/flamegraph/tsconfig.json grafana/flamegraph/yarn.lock  flamegraph/
RUN cd flamegraph && yarn install

ENV NODE_ENV production
COPY grafana/phlare-datasource/src/ phlare-datasource/src/
COPY grafana/flamegraph/src/ flamegraph/src/
RUN cd phlare-datasource && yarn build
RUN cd flamegraph && yarn build

FROM golang:1.19.2-alpine as go-builder

WORKDIR /build
COPY grafana/phlare-datasource/go.mod grafana/phlare-datasource/go.sum grafana/phlare-datasource/Magefile.go grafana/phlare-datasource/
COPY grafana/phlare-datasource/pkg/ grafana/phlare-datasource/pkg/
COPY grafana/phlare-datasource/src/ grafana/phlare-datasource/src/
COPY pkg/ pkg/
COPY go.mod ./
COPY go.sum ./

RUN cd grafana/phlare-datasource && go mod verify && go install github.com/magefile/mage@v1.13.0 && mage -v

FROM aocenas/grafana:explore-experiment-2

ENV GF_DEFAULT_APP_MODE=development

COPY --from=js-builder /grafana/phlare-datasource/dist/ /var/lib/grafana/plugins/phlare-datasource/
COPY --from=js-builder /grafana/flamegraph/dist/ /var/lib/grafana/plugins/flamegraph/
COPY --from=go-builder /build/grafana/phlare-datasource/dist/ /var/lib/grafana/plugins/phlare-datasource/
