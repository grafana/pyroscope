FROM node:24@sha256:bb20cf73b3ad7212834ec48e2174cdcb5775f6550510a5336b842ae32741ce6c AS builder

WORKDIR /pyroscope/ui
COPY ui/package.json ui/yarn.lock ui/.yarnrc.yml ./
COPY ui/.yarn ./.yarn
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
