FROM node:26.12.1@sha256:a98e6fda54fdbb22d4238054cca9a2a113bcd1191c09193a535aae22f400aa8e AS builder

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
