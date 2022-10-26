FROM openjdk:11-slim-bullseye

WORKDIR /app

COPY . .

RUN ./gradlew clean build

EXPOSE 8080

CMD ["./gradlew", "bootRun"]
