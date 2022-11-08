FROM openjdk:11-slim-bullseye

RUN apt-get update && apt install -y openjdk-11-dbg

WORKDIR /app

COPY . .

RUN ./gradlew clean build

EXPOSE 8080

CMD ["./gradlew", "bootRun"]
