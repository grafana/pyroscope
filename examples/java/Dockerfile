FROM openjdk:11.0.11-jdk

WORKDIR /opt/app
RUN git clone https://github.com/pyroscope-io/pyroscope-java.git
RUN cd pyroscope-java && \
    ./gradlew shadowJar && \
    cp agent/build/libs/pyroscope.jar /opt/app/pyroscope.jar

COPY Main.java ./Main.java

ENV PYROSCOPE_APPLICATION_NAME=simple.java.app
ENV PYROSCOPE_PROFILING_INTERVAL=10ms
ENV PYROSCOPE_PROFILER_EVENT=cpu
ENV PYROSCOPE_UPLOAD_INTERVAL=10s
ENV PYROSCOPE_LOG_LEVEL=debug
ENV PYROSCOPE_SERVER_ADDRESS=http://pyroscope:4040

RUN javac Main.java

CMD ["java", "-XX:-Inline", "-javaagent:pyroscope.jar", "Main"]
