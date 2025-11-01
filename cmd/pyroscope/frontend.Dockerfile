FROM node:24@sha256:34af25027ee1b8bffd482ba995ec1e577fbd398db87beb4c60b80c2c9c025127 AS builder

WORKDIR /pyroscope
COPY yarn.lock package.json tsconfig.json ./
RUN --mount=type=cache,target=/usr/local/share/.cache/yarn/v6 yarn --frozen-lockfile
COPY scripts/webpack ./scripts/webpack/
COPY public/app ./public/app
COPY public/templates ./public/templates
RUN yarn build

# Usage: docker build  -f cmd/pyroscope/frontend.Dockerfile --output=public/build .
FROM scratch
COPY --from=builder /pyroscope/public/build /