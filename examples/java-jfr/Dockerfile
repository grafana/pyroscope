FROM openjdk:17-slim-bullseye

WORKDIR /opt/agent

RUN apt-get update && apt-get install ca-certificates -y && update-ca-certificates && apt-get install -y git
RUN git clone https://github.com/pyroscope-io/pyroscope-java.git
RUN cd pyroscope-java && \
    ./gradlew shadowJar && \
    cp agent/build/libs/pyroscope.jar ../pyroscope.jar

COPY Main.java ./

RUN javac Main.java

CMD [ "java", "-javaagent:pyroscope.jar", "Main"]
