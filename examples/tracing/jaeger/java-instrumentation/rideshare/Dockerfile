FROM openjdk:11-slim-bullseye

WORKDIR /opt/app


COPY gradlew .
COPY gradle gradle
RUN ./gradlew --no-daemon 


COPY build.gradle.kts settings.gradle.kts  ./
RUN ./gradlew getDeps --no-daemon
ADD https://github.com/open-telemetry/opentelemetry-java-instrumentation/releases/download/v1.17.0/opentelemetry-javaagent.jar opentelemetry-javaagent.jar 
# ADD https://s01.oss.sonatype.org/content/repositories/snapshots/io/pyroscope/otel/0.10.1.2-SNAPSHOT/otel-0.10.1.2-20220907.093035-3.jar pyroscope-otel.jar
ADD https://repo1.maven.org/maven2/io/pyroscope/otel/0.10.1.3/otel-0.10.1.3.jar pyroscope-otel.jar

COPY src src
RUN ./gradlew assemble --no-daemon

ENV OTEL_SERVICE_NAME=ride-sharing-app-java-instrumentation
ENV OTEL_METRICS_EXPORTER=none
ENV OTEL_TRACES_EXPORTER=jaeger
ENV OTEL_EXPORTER_JAEGER_ENDPOINT=http://localhost:14250
ENV OTEL_TRACES_SAMPLER=always_on
ENV OTEL_PROPAGATORS=tracecontext
ENV OTEL_JAVAAGENT_EXTENSIONS=./pyroscope-otel.jar

ENV OTEL_PYROSCOPE_ADD_PROFILE_URL=true
ENV OTEL_PYROSCOPE_ADD_PROFILE_BASELINE_URL=true
ENV OTEL_PYROSCOPE_START_PROFILING=true
ENV OTEL_PYROSCOPE_ENDPOINT=http://localhost:4040

ENV PYROSCOPE_APPLICATION_NAME=ride-sharing-app-java-instrumentation
ENV PYROSCOPE_FORMAT=jfr
ENV PYROSCOPE_PROFILER_EVENT=itimer
ENV PYROSCOPE_SERVER_ADDRESS=http://localhost:4040
ENV PYROSCOPE_LABELS="region=eu-north"
ENV REGION=eu-north

CMD [ "java",  "-javaagent:./opentelemetry-javaagent.jar", "-jar", "./build/libs/rideshare-instrumentation-1.0-SNAPSHOT.jar"]


