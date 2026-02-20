FROM node:22@sha256:cd7bcd2e7a1e6f72052feb023c7f6b722205d3fcab7bbcbd2d1bfdab10b1e935 AS builder

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