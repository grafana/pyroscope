FROM openjdk:11-jdk-slim

RUN apt-get install libstdc++6

WORKDIR /app

COPY . .

RUN ./gradlew clean build

EXPOSE 8080

CMD ["./gradlew","run"]
