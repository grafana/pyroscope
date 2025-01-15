FROM gradle:8.5-jdk17 AS build
WORKDIR /app
COPY . .
RUN gradle shadowJar

FROM eclipse-temurin:17-jre
WORKDIR /app

ADD https://github.com/open-telemetry/opentelemetry-java-instrumentation/releases/download/v1.17.0/opentelemetry-javaagent.jar opentelemetry-javaagent.jar

COPY --from=build /app/build/libs/*.jar app.jar

ENV JAVA_TOOL_OPTIONS="-javaagent:/app/opentelemetry-javaagent.jar"

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

ADD https://repo1.maven.org/maven2/io/pyroscope/otel/0.11.0/otel-0.11.0.jar pyroscope-otel.jar

EXPOSE 5000
CMD ["java", "-jar", "app.jar"]
