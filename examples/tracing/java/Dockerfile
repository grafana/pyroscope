FROM --platform=$BUILDPLATFORM sapmachine:17-jdk-headless as builder

WORKDIR /opt/app

RUN apt-get update && apt-get install ca-certificates -y && update-ca-certificates 



COPY gradlew .
COPY gradle gradle
RUN ./gradlew

COPY build.gradle.kts settings.gradle.kts ./
RUN ./gradlew dependencies --no-daemon

COPY src src
RUN ./gradlew assemble --no-daemon


FROM sapmachine:17-jdk-headless

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

ENV OTEL_JAVAAGENT_EXTENSIONS=./pyroscope-otel.jar

ENV OTEL_PYROSCOPE_ADD_PROFILE_URL=false
ENV OTEL_PYROSCOPE_ADD_PROFILE_BASELINE_URL=false
ENV OTEL_PYROSCOPE_START_PROFILING=true

COPY --from=builder /opt/app/build/libs/rideshare-1.0-SNAPSHOT.jar /opt/app/build/libs/rideshare-1.0-SNAPSHOT.jar

WORKDIR /opt/app

ADD https://github.com/open-telemetry/opentelemetry-java-instrumentation/releases/download/v2.15.0/opentelemetry-javaagent.jar opentelemetry-javaagent.jar
ADD https://github.com/grafana/pyroscope-java/releases/download/v2.1.2/pyroscope.jar pyroscope.jar
ADD https://github.com/grafana/otel-profiling-java/releases/download/v1.0.4/pyroscope-otel.jar pyroscope-otel.jar

EXPOSE 5000

CMD ["java", "-Dserver.port=5000", "-javaagent:./pyroscope.jar", "-javaagent:./opentelemetry-javaagent.jar", "-jar", "./build/libs/rideshare-1.0-SNAPSHOT.jar" ]
