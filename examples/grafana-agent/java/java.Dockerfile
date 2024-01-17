FROM openjdk:17-jdk-slim

ADD ./FastSlow.java /FastSlow.java
RUN javac FastSlow.java

CMD ["java", "FastSlow"]
