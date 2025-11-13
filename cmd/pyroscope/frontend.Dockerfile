FROM node:24@sha256:7f80506b8225bcce2ce8202b1026fcde8f0bfb716b1b833f20250d79d4463276 AS builder

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