FROM --platform=$BUILDPLATFORM sapmachine:17-jdk-headless as builder

WORKDIR /opt/app

RUN apt-get update && apt-get install ca-certificates -y && update-ca-certificates



COPY gradlew .
COPY gradle gradle
RUN ./gradlew

COPY build.gradle.kts settings.gradle.kts ./
RUN ./gradlew dependencies --no-daemon

COPY src src
RUN ./gradlew assemble --no-daemon


FROM sapmachine:17-jdk-headless

RUN apt-get update && apt-get install ca-certificates -y && update-ca-certificates

COPY --from=builder /opt/app/build/libs/rideshare-1.0-SNAPSHOT.jar /opt/app/build/libs/rideshare-1.0-SNAPSHOT.jar

WORKDIR /opt/app

EXPOSE 5000

CMD ["java", "-Dserver.port=5000", "-jar", "./build/libs/rideshare-1.0-SNAPSHOT.jar"]
