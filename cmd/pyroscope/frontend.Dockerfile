FROM node:26.12.1@sha256:97fa0f1b59035c6072721a88d55be1371c7cb1cc942b521e1cc5c72ec87655f4 AS builder

WORKDIR /pyroscope/ui
COPY ui/package.json ui/yarn.lock ui/.yarnrc.yml ui/.npmrc ./
COPY ui/install-yarn.sh ./
RUN sh ./install-yarn.sh && yarn install --immutable
COPY ui/index.html ui/vite.config.ts ./
COPY ui/tsconfig*.json ./
COPY ui/src ./src
COPY ui/public ./public
RUN yarn build
# Output lands at /pyroscope/ui/dist/ (set by build.outDir in vite.config.ts)

# Usage: docker build -f cmd/pyroscope/frontend.Dockerfile --output=ui/dist .
FROM scratch
COPY --from=builder /pyroscope/ui/dist /
