FROM openjdk:17-slim-bullseye

WORKDIR /opt/app

RUN apt-get update && apt-get install ca-certificates -y && update-ca-certificates && apt-get install -y git
RUN git clone https://github.com/pyroscope-io/pyroscope-java.git && \
    cd pyroscope-java && \
    git checkout v0.6.0 && \
    ./gradlew shadowJar && \
    cp agent/build/libs/pyroscope.jar /opt/app/pyroscope.jar

COPY Main.java ./

RUN javac Main.java

ENV PYROSCOPE_APPLICATION_NAME=fibonacci.java.push.app
ENV PYROSCOPE_FORMAT=jfr
ENV PYROSCOPE_PROFILING_INTERVAL=10ms
ENV PYROSCOPE_PROFILER_EVENT=cpu
ENV PYROSCOPE_PROFILER_LOCK=1
ENV PYROSCOPE_PROFILER_ALLOC=1
ENV PYROSCOPE_UPLOAD_INTERVAL=10s
ENV PYROSCOPE_LOG_LEVEL=debug
ENV PYROSCOPE_SERVER_ADDRESS=http://localhost:4040

CMD ["java", "-XX:-Inline", "-javaagent:pyroscope.jar", "Main"]
