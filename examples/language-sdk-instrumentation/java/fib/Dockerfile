FROM openjdk:17-slim-bullseye

WORKDIR /opt/app

RUN apt-get update && apt-get install ca-certificates -y && update-ca-certificates && apt-get install -y git

ADD https://github.com/grafana/pyroscope-java/releases/download/v0.15.2/pyroscope.jar /opt/app/pyroscope.jar

COPY Main.java ./

RUN javac Main.java

ENV PYROSCOPE_APPLICATION_NAME=fibonacci.java.push.app
ENV PYROSCOPE_FORMAT=jfr
ENV PYROSCOPE_PROFILING_INTERVAL=10ms
ENV PYROSCOPE_PROFILER_EVENT=cpu
ENV PYROSCOPE_PROFILER_LOCK=10ms
ENV PYROSCOPE_PROFILER_ALLOC=512k
ENV PYROSCOPE_UPLOAD_INTERVAL=15s
ENV PYROSCOPE_LOG_LEVEL=debug
ENV PYROSCOPE_SERVER_ADDRESS=http://localhost:4040

CMD ["java", "-XX:-Inline", "-javaagent:pyroscope.jar", "Main"]
