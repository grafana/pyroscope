FROM openjdk:11-slim-bullseye

WORKDIR /opt/app


COPY gradlew .
COPY gradle gradle
RUN ./gradlew

COPY build.gradle.kts settings.gradle.kts  .
COPY src src
RUN ./gradlew assemble --no-daemon


CMD ["java",  "-jar", "./build/libs/rideshare-1.0-SNAPSHOT.jar"]
