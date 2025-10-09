FROM eclipse-temurin:11-jdk AS build
WORKDIR /app
COPY FastSlow.java .
RUN javac FastSlow.java

FROM ibm-semeru-runtimes:open-11-jdk
WORKDIR /app
COPY --from=build /app/FastSlow.class .

CMD ["java", "FastSlow", "-XX:+FlightRecorder"]
