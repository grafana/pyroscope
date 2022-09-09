FROM openjdk:11-slim-bullseye

WORKDIR /opt/app

COPY gradlew .
COPY gradle gradle
RUN ./gradlew --no-daemon

COPY build.gradle.kts settings.gradle.kts  ./
RUN ./gradlew getDeps --no-daemon

COPY src src
RUN ./gradlew assemble --no-daemon


CMD ["java",  "-jar", "./build/libs/rideshare-manual-1.0-SNAPSHOT.jar"]
