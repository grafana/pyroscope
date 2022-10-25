FROM openjdk:11-jdk-slim

WORKDIR /app

COPY . .

RUN ./gradlew clean build

EXPOSE 8080

CMD ["./gradlew","run"]
