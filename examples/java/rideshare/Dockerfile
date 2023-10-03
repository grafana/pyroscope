FROM openjdk:17-slim-bullseye

WORKDIR /opt/app

RUN apt-get update && apt-get install ca-certificates -y && update-ca-certificates 



COPY gradlew .
COPY gradle gradle
RUN ./gradlew

COPY build.gradle.kts settings.gradle.kts ./
COPY src src
RUN ./gradlew assemble --no-daemon

ENV PYROSCOPE_APPLICATION_NAME=rideshare.java.push.app
ENV PYROSCOPE_FORMAT=jfr
ENV PYROSCOPE_PROFILING_INTERVAL=10ms
ENV PYROSCOPE_PROFILER_EVENT=itimer
ENV PYROSCOPE_PROFILER_LOCK=10ms
ENV PYROSCOPE_PROFILER_ALLOC=512k
ENV PYROSCOPE_UPLOAD_INTERVAL=15s
ENV PYROSCOPE_LOG_LEVEL=debug
ENV PYROSCOPE_SERVER_ADDRESS=http://localhost:4040
EXPOSE 5000

ADD https://github.com/pyroscope-io/pyroscope-java/releases/download/v0.8.0/pyroscope.jar /opt/app/pyroscope.jar

CMD ["java", "-Dserver.port=5000",  "-javaagent:pyroscope.jar", "-jar", "./build/libs/rideshare-1.0-SNAPSHOT.jar" ]
