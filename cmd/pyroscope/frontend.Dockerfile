FROM node:24@sha256:3a09aa6354567619221ef6c45a5051b671f953f0a1924d1f819ffb236e520e6b AS builder

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