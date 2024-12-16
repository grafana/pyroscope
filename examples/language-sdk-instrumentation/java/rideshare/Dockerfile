FROM --platform=$BUILDPLATFORM openjdk:17-slim-bullseye as builder

WORKDIR /opt/app

RUN apt-get update && apt-get install ca-certificates -y && update-ca-certificates 



COPY gradlew .
COPY gradle gradle
RUN ./gradlew

COPY build.gradle.kts settings.gradle.kts ./
RUN ./gradlew dependencies --no-daemon

COPY src src
RUN ./gradlew assemble --no-daemon


FROM  openjdk:17-slim-bullseye

RUN apt-get update && apt-get install ca-certificates -y && update-ca-certificates

ENV PYROSCOPE_APPLICATION_NAME=rideshare.java.push.app
ENV PYROSCOPE_FORMAT=jfr
ENV PYROSCOPE_PROFILING_INTERVAL=10ms
ENV PYROSCOPE_PROFILER_EVENT=itimer
ENV PYROSCOPE_PROFILER_LOCK=10ms
ENV PYROSCOPE_PROFILER_ALLOC=512k
ENV PYROSCOPE_UPLOAD_INTERVAL=15s
ENV PYROSCOPE_LOG_LEVEL=debug
ENV PYROSCOPE_SERVER_ADDRESS=http://localhost:4040
ENV RIDESHARE_LISTEN_PORT=5000
EXPOSE 5000

COPY --from=builder /opt/app/build/libs/rideshare-1.0-SNAPSHOT.jar /opt/app/build/libs/rideshare-1.0-SNAPSHOT.jar

WORKDIR /opt/app

ADD https://github.com/grafana/pyroscope-java/releases/download/v0.15.2/pyroscope.jar /opt/app/pyroscope.jar

CMD sh -c "exec java -Dserver.port=${RIDESHARE_LISTEN_PORT} -javaagent:pyroscope.jar -jar ./build/libs/rideshare-1.0-SNAPSHOT.jar"
