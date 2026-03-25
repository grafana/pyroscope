FROM node:22@sha256:cd7bcd2e7a1e6f72052feb023c7f6b722205d3fcab7bbcbd2d1bfdab10b1e935 AS builder

WORKDIR /pyroscope/ui
COPY ui/package.json ui/yarn.lock ui/.yarnrc.yml ./
COPY ui/.yarn ./.yarn
RUN corepack enable && yarn install --immutable
COPY ui/index.html ui/vite.config.ts ./
COPY ui/tsconfig*.json ./
COPY ui/src ./src
COPY ui/public ./public
RUN yarn build
# Output lands at /pyroscope/public/build/ (set by build.outDir in vite.config.ts)

# Usage: docker build -f cmd/pyroscope/frontend.Dockerfile --output=public/build .
FROM scratch
COPY --from=builder /pyroscope/public/build /
