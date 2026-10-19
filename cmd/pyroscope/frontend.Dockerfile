FROM node:24@sha256:14078204b6bb268c107ceff6d3c43f45a888105e5c2b18b14c32b3409d93a1b0 AS builder

WORKDIR /pyroscope/ui
COPY ui/package.json ui/yarn.lock ui/.yarnrc.yml ui/.npmrc ./
RUN corepack enable && yarn install --immutable
COPY ui/index.html ui/vite.config.ts ./
COPY ui/tsconfig*.json ./
COPY ui/src ./src
COPY ui/public ./public
RUN yarn build
# Output lands at /pyroscope/ui/dist/ (set by build.outDir in vite.config.ts)

# Usage: docker build -f cmd/pyroscope/frontend.Dockerfile --output=ui/dist .
FROM scratch
COPY --from=builder /pyroscope/ui/dist /
