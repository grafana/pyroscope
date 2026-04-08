FROM sapmachine:17-jdk-headless

ADD ./FastSlow.java /FastSlow.java
RUN javac FastSlow.java

CMD ["java", "FastSlow"]
