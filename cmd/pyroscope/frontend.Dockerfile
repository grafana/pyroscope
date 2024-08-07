FROM node:18 AS builder
RUN apt-get update && apt-get install -y libpango1.0-dev libcairo2-dev
WORKDIR /pyroscope
COPY yarn.lock package.json tsconfig.json ./
RUN yarn --frozen-lockfile
COPY scripts/webpack ./scripts/webpack/
COPY public/app ./public/app
COPY public/templates ./public/templates
RUN yarn build

# Usage: docker build  -f cmd/pyroscope/frontend.Dockerfile --output=public/build .
FROM scratch
COPY --from=builder /pyroscope/public/build /