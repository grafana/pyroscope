FROM openjdk:11-slim-bullseye

WORKDIR /opt/app


# download gradle
COPY gradlew .
COPY gradle gradle
RUN ./gradlew --no-daemon

# download deps
COPY build.gradle.kts settings.gradle.kts  ./
RUN ./gradlew getDeps --no-daemon

# build
COPY src src
RUN ./gradlew assemble --no-daemon


CMD ["java",  "-jar", "./build/libs/rideshare-1.0-SNAPSHOT.jar"]
