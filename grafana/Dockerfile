FROM node:16-alpine3.15 as js-builder

ENV NODE_OPTIONS=--max_old_space_size=8000

WORKDIR /grafana

COPY grafana/fire-datasource/package.json grafana/fire-datasource/README.md grafana/fire-datasource/.prettierrc.js grafana/fire-datasource/tsconfig.json grafana/fire-datasource/yarn.lock  fire-datasource/
RUN cd fire-datasource && yarn install

COPY grafana/flamegraph/package.json grafana/flamegraph/README.md  grafana/flamegraph/.prettierrc.js grafana/flamegraph/tsconfig.json grafana/flamegraph/yarn.lock  flamegraph/
RUN cd flamegraph && yarn install

ENV NODE_ENV production
COPY grafana/fire-datasource/src/ fire-datasource/src/
COPY grafana/flamegraph/src/ flamegraph/src/
RUN cd fire-datasource && yarn build
RUN cd flamegraph && yarn build

FROM golang:1.19-alpine as go-builder

WORKDIR /build
COPY grafana/fire-datasource/go.mod grafana/fire-datasource/go.sum grafana/fire-datasource/Magefile.go grafana/fire-datasource/
COPY grafana/fire-datasource/pkg/ grafana/fire-datasource/pkg/
COPY grafana/fire-datasource/src/ grafana/fire-datasource/src/
COPY pkg/ pkg/
COPY go.mod ./
COPY go.sum ./

RUN cd grafana/fire-datasource && go mod verify && go install github.com/magefile/mage@v1.13.0 && mage -v

FROM aocenas/grafana:explore-experiment-2

ENV GF_DEFAULT_APP_MODE=development

COPY --from=js-builder /grafana/fire-datasource/dist/ /var/lib/grafana/plugins/fire-datasource/
COPY --from=js-builder /grafana/flamegraph/dist/ /var/lib/grafana/plugins/flamegraph/
COPY --from=go-builder /build/grafana/fire-datasource/dist/ /var/lib/grafana/plugins/fire-datasource/
