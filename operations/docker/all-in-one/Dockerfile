FROM grafana/pyroscope:1.6.1 as pyroscope

FROM grafana/grafana:11.1.0

ARG TARGETPLATFORM

USER root
RUN apk add --no-cache curl

ARG S6_OVERLAY_VERSION=3.2.0.0
# Machine independent
RUN true \
  && curl -Lo /tmp/s6-overlay-noarch.tar.xz "https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-noarch.tar.xz" \
  && echo -e "4b0c0907e6762814c31850e0e6c6762c385571d4656eb8725852b0b1586713b6  /tmp/s6-overlay-noarch.tar.xz" | grep "${TARGET}" | sha256sum -c - \
  && tar -C / -Jxpf /tmp/s6-overlay-noarch.tar.xz

# Machine depedendant
RUN true \
  && export TARGET=$(uname -m) \
  && curl -Lo /tmp/s6-overlay-${TARGET}.tar.xz "https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-${TARGET}.tar.xz" \
  && echo -e "868973e98210257bba725ff5b17aa092008c9a8e5174499e38ba611a8fc7e473  /tmp/s6-overlay-aarch64.tar.xz\nad982a801bd72757c7b1b53539a146cf715e640b4d8f0a6a671a3d1b560fe1e2  /tmp/s6-overlay-x86_64.tar.xz" | grep "${TARGET}" | sha256sum -c - \
  && tar -C / -Jxpf /tmp/s6-overlay-${TARGET}.tar.xz

# Copy over pyroscope binaries
COPY --from=pyroscope /usr/bin/pyroscope /usr/bin/pyroscope
COPY --from=pyroscope /usr/bin/profilecli /usr/bin/profilecli
VOLUME /var/lib/pyroscope

# Ensure services are started:
COPY ./s6-rc.d/ /etc/s6-overlay/s6-rc.d

ENV S6_KEEP_ENV=1
ENTRYPOINT ["/init"]
